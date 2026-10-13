package wasm

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"sync"

	"euromni/internal/abi"
	"euromni/internal/store"

	"github.com/bytecodealliance/wasmtime-go/v36"
)

var (
	ErrRuntimeClosed       = errors.New("wasm runtime is closed")
	ErrModuleAlreadyLoaded = errors.New("wasm module is already loaded")
	ErrModuleNotInstalled  = errors.New("wasm module is not installed for this shop")
	ErrModuleHashMissing   = errors.New("wasm module hash is missing")
	ErrModuleHashMismatch  = errors.New("wasm module hash does not match")
	ErrUnsupportedABI      = errors.New("wasm module ABI version is not supported")
	ErrModuleNeedsReload   = errors.New("installed wasm module changed; reload it before execution")
	ErrUnknownCapability   = errors.New("wasm module has an unknown capability")
)

const (
	guestCallModule       = "euromni"
	guestCallName         = "call"
	maxGuestResponseBytes = 1 << 20
	guestFuelLimit        = 10_000_000
	guestMemoryLimit      = 64 << 20
)

// Runtime is the Wasmtime-backed sandbox for translated marketplace modules.
// Modules have no host access unless an explicit capability is granted.
type Runtime struct {
	Host         *Host
	engine       *wasmtime.Engine
	modules      map[string]*wasmtime.Module
	moduleNames  map[string]string
	moduleHashes map[string][]byte
	mu           sync.RWMutex
	closed       bool
}

func New() *Runtime {
	return NewRuntime(nil)
}

func NewRuntime(host *Host) *Runtime {
	config := wasmtime.NewConfig()
	config.SetConsumeFuel(true)
	return &Runtime{
		Host:         host,
		engine:       wasmtime.NewEngineWithConfig(config),
		modules:      make(map[string]*wasmtime.Module),
		moduleNames:  make(map[string]string),
		moduleHashes: make(map[string][]byte),
	}
}

// Load compiles and registers a named WebAssembly module without instantiating it.
func (r *Runtime) Load(name string, wasm []byte) error {
	return r.load(manualModuleKey(name), name, wasm, nil, false)
}

func (r *Runtime) LoadInstalled(ctx context.Context, moduleStore *store.Store, shopID, moduleID string, wasm []byte) error {
	return r.loadInstalled(ctx, moduleStore, shopID, moduleID, wasm, false)
}

func (r *Runtime) ReloadInstalled(ctx context.Context, moduleStore *store.Store, shopID, moduleID string, wasm []byte) error {
	return r.loadInstalled(ctx, moduleStore, shopID, moduleID, wasm, true)
}

func (r *Runtime) loadInstalled(ctx context.Context, moduleStore *store.Store, shopID, moduleID string, wasm []byte, replace bool) error {
	if moduleStore == nil {
		return errors.New("module store is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	installed, err := moduleStore.GetInstalledModule(ctx, shopID, moduleID)
	if err != nil {
		return err
	}
	return r.registerInstalled(installed, shopID, moduleID, wasm, replace)
}

func (r *Runtime) loadInstalledRecord(installed *store.InstalledModule, shopID, moduleID string, wasm []byte) error {
	return r.registerInstalled(installed, shopID, moduleID, wasm, false)
}

func (r *Runtime) reloadInstalledRecord(installed *store.InstalledModule, shopID, moduleID string, wasm []byte) error {
	return r.registerInstalled(installed, shopID, moduleID, wasm, true)
}

func (r *Runtime) registerInstalled(installed *store.InstalledModule, shopID, moduleID string, wasm []byte, replace bool) error {
	if installed == nil || installed.ID != moduleID {
		return ErrModuleNotInstalled
	}
	if installed.ABIVersion != abi.Version {
		return ErrUnsupportedABI
	}
	if len(installed.WASMSHA256) != sha256.Size {
		return ErrModuleHashMissing
	}
	actualHash := sha256.Sum256(wasm)
	if subtle.ConstantTimeCompare(installed.WASMSHA256, actualHash[:]) != 1 {
		return ErrModuleHashMismatch
	}
	capabilities, err := storedCapabilities(installed.Capabilities)
	if err != nil {
		return err
	}
	if _, err := trustedContext(shopID, moduleID, capabilities...); err != nil {
		return err
	}
	return r.load(installedModuleKey(shopID, moduleID), installed.Name, wasm, installed.WASMSHA256, replace)
}

func manualModuleKey(name string) string { return "manual:" + name }

func installedModuleKey(shopID, moduleID string) string {
	return "installed:" + shopID + ":" + moduleID
}

func storedCapabilities(names []string) ([]Capability, error) {
	capabilities := make([]Capability, 0, len(names))
	seen := make(map[Capability]struct{}, len(names))
	for _, name := range names {
		capability := Capability(name)
		switch capability {
		case CapCatalog, CapCart, CapOrder, CapCustomer, CapPayment, CapContent, CapHooks, CapAdmin, CapHTTPFetch, CapModuleKV:
		default:
			return nil, fmt.Errorf("%w: %s", ErrUnknownCapability, name)
		}
		if _, exists := seen[capability]; exists {
			return nil, fmt.Errorf("duplicate wasm capability: %s", name)
		}
		seen[capability] = struct{}{}
		capabilities = append(capabilities, capability)
	}
	return capabilities, nil
}

func (r *Runtime) load(key, name string, wasm, installedHash []byte, replace bool) error {
	if r == nil || r.engine == nil {
		return errors.New("wasm runtime is not initialized")
	}
	if name == "" || strings.TrimSpace(name) != name {
		return errors.New("wasm module name is required")
	}
	if len(wasm) == 0 {
		return errors.New("wasm module is empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrRuntimeClosed
	}
	previous, exists := r.modules[key]
	if exists && !replace {
		return ErrModuleAlreadyLoaded
	}
	module, err := wasmtime.NewModule(r.engine, wasm)
	if err != nil {
		return fmt.Errorf("compile wasm module %q: %w", name, err)
	}
	if exists {
		previous.Close()
	}
	r.modules[key] = module
	if len(installedHash) == sha256.Size {
		r.moduleNames[key] = name
		r.moduleHashes[key] = append([]byte(nil), installedHash...)
	}
	return nil
}

// Run executes only a module loaded from an installed shop_modules record.
func (r *Runtime) Run(ctx context.Context, moduleStore *store.Store, shopID, moduleID, exportName string) ([]interface{}, error) {
	return r.runInstalled(ctx, moduleStore, shopID, moduleID, exportName, nil)
}

// RunWithInput invokes an installed guest export with a validated ABI envelope in linear memory.
func (r *Runtime) RunWithInput(ctx context.Context, moduleStore *store.Store, shopID, moduleID, exportName string, input []byte) ([]interface{}, error) {
	if err := validateGuestInput(input); err != nil {
		return nil, err
	}
	return r.runInstalled(ctx, moduleStore, shopID, moduleID, exportName, input)
}

func (r *Runtime) runInstalled(ctx context.Context, moduleStore *store.Store, shopID, moduleID, exportName string, input []byte) ([]interface{}, error) {
	if r == nil {
		return nil, errors.New("wasm runtime is not initialized")
	}
	key := installedModuleKey(shopID, moduleID)
	r.mu.RLock()
	_, loaded := r.modules[key]
	loadedName := r.moduleNames[key]
	loadedHash := append([]byte(nil), r.moduleHashes[key]...)
	r.mu.RUnlock()
	if !loaded || len(loadedHash) != sha256.Size {
		return nil, ErrModuleNotInstalled
	}
	if moduleStore == nil {
		return nil, errors.New("module store is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	installed, err := moduleStore.GetInstalledModule(ctx, shopID, moduleID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrModuleNotInstalled
	}
	if err != nil {
		return nil, err
	}
	if installed == nil || installed.ID != moduleID {
		return nil, ErrModuleNotInstalled
	}
	if installed.ABIVersion != abi.Version {
		return nil, ErrUnsupportedABI
	}
	if installed.Name != loadedName || len(installed.WASMSHA256) != sha256.Size || subtle.ConstantTimeCompare(installed.WASMSHA256, loadedHash) != 1 {
		return nil, ErrModuleNeedsReload
	}
	capabilities, err := storedCapabilities(installed.Capabilities)
	if err != nil {
		return nil, err
	}
	trusted, err := trustedContext(shopID, moduleID, capabilities...)
	if err != nil {
		return nil, err
	}
	if input != nil {
		return r.runWithInput(ctx, trusted, key, exportName, input)
	}
	return r.run(ctx, trusted, key, exportName)
}

func validateGuestInput(input []byte) error {
	if len(input) == 0 || len(input) > abi.MaxCallBytes {
		return errors.New("guest ABI input must be non-empty and within the call size limit")
	}
	if _, err := abi.ParseEnvelope(input); err != nil {
		return fmt.Errorf("invalid guest ABI input: %w", err)
	}
	return nil
}

// runWithContext is reserved for in-package tests and trusted host orchestration.
func (r *Runtime) runWithContext(ctx context.Context, tc TrustedContext, moduleName, exportName string) ([]interface{}, error) {
	return r.run(ctx, tc, manualModuleKey(moduleName), exportName)
}

func (r *Runtime) run(ctx context.Context, tc TrustedContext, key, exportName string) ([]interface{}, error) {
	return r.runEntry(ctx, tc, key, exportName, nil)
}

func (r *Runtime) runWithInput(ctx context.Context, tc TrustedContext, key, exportName string, input []byte) ([]interface{}, error) {
	if err := validateGuestInput(input); err != nil {
		return nil, err
	}
	return r.runEntry(ctx, tc, key, exportName, input)
}

func (r *Runtime) runEntry(ctx context.Context, tc TrustedContext, key, exportName string, input []byte) ([]interface{}, error) {
	if r == nil || r.engine == nil {
		return nil, errors.New("wasm runtime is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if key == "" || exportName == "" || strings.TrimSpace(exportName) != exportName {
		return nil, errors.New("wasm module and export names are required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, ErrRuntimeClosed
	}
	module, exists := r.modules[key]
	if !exists {
		return nil, ErrModuleNotInstalled
	}
	store := wasmtime.NewStore(r.engine)
	defer store.Close()
	store.Limiter(guestMemoryLimit, 10_000, 1, 1, 1)
	if err := store.SetFuel(guestFuelLimit); err != nil {
		return nil, fmt.Errorf("configure wasm fuel: %w", err)
	}

	linker := wasmtime.NewLinker(r.engine)
	defer linker.Close()
	if err := linker.DefineFunc(store, guestCallModule, guestCallName, func(caller *wasmtime.Caller, inputPtr, inputLen, outputPtr, outputCapacity int32) int32 {
		return r.callHost(ctx, tc, caller, inputPtr, inputLen, outputPtr, outputCapacity)
	}); err != nil {
		return nil, fmt.Errorf("link EUROMNI ABI: %w", err)
	}
	instance, err := linker.Instantiate(store, module)
	if err != nil {
		return nil, fmt.Errorf("instantiate wasm module %q: %w", key, err)
	}
	guest := instance.GetFunc(store, exportName)
	if guest == nil {
		return nil, fmt.Errorf("wasm export %q is missing or is not a function", exportName)
	}
	var result interface{}
	if input == nil {
		result, err = guest.Call(store)
	} else {
		exportedMemory := instance.GetExport(store, "memory")
		if exportedMemory == nil {
			return nil, errors.New("wasm memory export is missing")
		}
		defer exportedMemory.Close()
		memory := exportedMemory.Memory()
		if memory == nil {
			return nil, errors.New("wasm memory export is invalid")
		}
		linearMemory := memory.UnsafeData(store)
		if len(input) > len(linearMemory) {
			return nil, errors.New("guest ABI input does not fit in linear memory")
		}
		copy(linearMemory, input)
		result, err = guest.Call(store, int32(0), int32(len(input)))
	}
	if err != nil {
		return nil, fmt.Errorf("run wasm export %q: %w", exportName, err)
	}
	switch values := result.(type) {
	case nil:
		return nil, nil
	case []wasmtime.Val:
		results := make([]interface{}, len(values))
		for i := range values {
			results[i] = values[i].Get()
		}
		return results, nil
	default:
		return []interface{}{values}, nil
	}
}

func (r *Runtime) callHost(ctx context.Context, tc TrustedContext, caller *wasmtime.Caller, inputPtr, inputLen, outputPtr, outputCapacity int32) int32 {
	if caller == nil || inputLen < 0 || outputCapacity < 0 || outputCapacity > maxGuestResponseBytes {
		return -1
	}
	exportedMemory := caller.GetExport("memory")
	if exportedMemory == nil {
		return -1
	}
	defer exportedMemory.Close()
	memory := exportedMemory.Memory()
	if memory == nil {
		return -1
	}
	linearMemory := memory.UnsafeData(caller)
	input, ok := guestMemoryRange(linearMemory, inputPtr, inputLen)
	if !ok {
		return -1
	}
	output, ok := guestMemoryRange(linearMemory, outputPtr, outputCapacity)
	if !ok {
		return -1
	}
	response := r.Host.Invoke(ctx, tc, input)
	if len(response) > len(output) {
		return -1
	}
	copy(output, response)
	return int32(len(response))
}

func guestMemoryRange(memory []byte, offset, length int32) ([]byte, bool) {
	if offset < 0 || length < 0 {
		return nil, false
	}
	start := uint64(offset)
	count := uint64(length)
	if start > uint64(len(memory)) || count > uint64(len(memory))-start {
		return nil, false
	}
	return memory[int(start):int(start+count)], true
}

func (r *Runtime) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	for name, module := range r.modules {
		module.Close()
		delete(r.modules, name)
	}
	if r.engine != nil {
		r.engine.Close()
	}
}
