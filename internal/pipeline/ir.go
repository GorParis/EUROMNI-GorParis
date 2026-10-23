package pipeline

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"euromni/internal/abi"
	"euromni/internal/buildmeta"
	"euromni/internal/wasm"

	"github.com/bytecodealliance/wasmtime-go/v36"
)

const IRVersion = buildmeta.IRVersion

const (
	WooCommerceSortingModuleName      = "woocommerce-extra-product-sorting-options"
	WooCommerceSortingModuleVersion   = "2.11.0"
	WooCommerceSortingSourceSHA256    = "aaf1a1a13ec7e0465a4c451b952aba79626be3288104c18122c47bf01c81306f"
	WooCommerceSortingSourceReference = "woocommerce-extra-product-sorting-options@bc5829d4d2383ae956869386eb40e772cf4a76a0"
)

const (
	irMemoryPages     = 17
	irOutputOffset    = abi.MaxCallBytes
	irOutputCapacity  = 1 << 20
	wasmIRHashSection = "euromni.ir.sha256"
)

type ModuleIR struct {
	Version    string           `json:"version"`
	Platform   string           `json:"platform"`
	Module     string           `json:"module"`
	Provenance SourceProvenance `json:"provenance"`
	Triggers   []IRTrigger      `json:"triggers"`
	Operations []IROperation    `json:"operations"`
}

type SourceProvenance struct {
	SourceSHA256    string `json:"source_sha256"`
	ArchiveSHA256   string `json:"archive_sha256"`
	SourceReference string `json:"source_reference,omitempty"`
	ModuleVersion   string `json:"module_version,omitempty"`
	PlatformVersion string `json:"platform_version"`
	Adapter         string `json:"adapter"`
	AdapterVersion  string `json:"adapter_version"`
}

type IRTrigger struct {
	Kind   string            `json:"kind"`
	Name   string            `json:"name"`
	Fields map[string]string `json:"fields,omitempty"`
}

type IRValue struct {
	Literal      json.RawMessage `json:"literal,omitempty"`
	TriggerField string          `json:"trigger_field,omitempty"`
}

type IROperation struct {
	Call  string             `json:"call"`
	Input map[string]IRValue `json:"input"`
}

type ValidatedIR struct {
	ModuleIR
	Capabilities []string `json:"capabilities"`
}

func NewSourceProvenance(source []byte, provenance SourceProvenance) (SourceProvenance, error) {
	if len(source) == 0 {
		return SourceProvenance{}, errors.New("source bytes are required for provenance")
	}
	digest := sha256.Sum256(source)
	computedHash := hex.EncodeToString(digest[:])
	if provenance.SourceSHA256 != "" && provenance.SourceSHA256 != computedHash {
		return SourceProvenance{}, errors.New("provided source hash does not match source bytes")
	}
	provenance.SourceSHA256 = computedHash
	return provenance, nil
}

func NewPackageProvenance(source, archive []byte, provenance SourceProvenance) (SourceProvenance, error) {
	if len(archive) == 0 {
		return SourceProvenance{}, errors.New("source archive bytes are required for provenance")
	}
	provenance, err := NewSourceProvenance(source, provenance)
	if err != nil {
		return SourceProvenance{}, err
	}
	digest := sha256.Sum256(archive)
	archiveHash := hex.EncodeToString(digest[:])
	if provenance.ArchiveSHA256 != "" && provenance.ArchiveSHA256 != archiveHash {
		return SourceProvenance{}, errors.New("provided archive hash does not match archive bytes")
	}
	provenance.ArchiveSHA256 = archiveHash
	return provenance, nil
}

func (p SourceProvenance) Validate(platform string) error {
	if p.Adapter != platform || strings.TrimSpace(p.Adapter) == "" {
		return errors.New("source adapter must match the IR platform")
	}
	if strings.TrimSpace(p.PlatformVersion) == "" || strings.TrimSpace(p.PlatformVersion) != p.PlatformVersion {
		return errors.New("source platform version is required and must be trimmed")
	}
	if strings.TrimSpace(p.AdapterVersion) == "" || strings.TrimSpace(p.AdapterVersion) != p.AdapterVersion {
		return errors.New("source adapter version is required and must be trimmed")
	}
	if !buildmeta.IsApprovedAdapter(platform, p.Adapter, p.AdapterVersion) {
		return errors.New("source adapter version is not approved")
	}
	if p.ModuleVersion != "" && strings.TrimSpace(p.ModuleVersion) != p.ModuleVersion {
		return errors.New("source module version must be trimmed")
	}
	if p.SourceReference != "" && strings.TrimSpace(p.SourceReference) != p.SourceReference {
		return errors.New("source reference must be trimmed")
	}
	digest, err := hex.DecodeString(p.SourceSHA256)
	if err != nil || len(digest) != sha256.Size || hex.EncodeToString(digest) != p.SourceSHA256 {
		return errors.New("source SHA-256 must be 64 lowercase hexadecimal characters")
	}
	archiveDigest, err := hex.DecodeString(p.ArchiveSHA256)
	if err != nil || len(archiveDigest) != sha256.Size || hex.EncodeToString(archiveDigest) != p.ArchiveSHA256 {
		return errors.New("archive SHA-256 must be 64 lowercase hexadecimal characters")
	}
	return nil
}

func (p SourceProvenance) VerifySource(source []byte) error {
	if len(source) == 0 {
		return errors.New("source bytes are required for provenance verification")
	}
	digest := sha256.Sum256(source)
	if hex.EncodeToString(digest[:]) != p.SourceSHA256 {
		return errors.New("source bytes do not match provenance hash")
	}
	return nil
}

func (p SourceProvenance) VerifyArchive(archive []byte) error {
	if len(archive) == 0 {
		return errors.New("archive bytes are required for provenance verification")
	}
	digest := sha256.Sum256(archive)
	if hex.EncodeToString(digest[:]) != p.ArchiveSHA256 {
		return errors.New("archive bytes do not match provenance hash")
	}
	return nil
}

func LowerPHPAnalysisToIR(module string, analysis *PHPAnalysis, provenance SourceProvenance) (ModuleIR, error) {
	if analysis == nil {
		return ModuleIR{}, errors.New("PHP analysis is required")
	}
	if provenance.SourceSHA256 != analysis.SourceSHA256 {
		return ModuleIR{}, errors.New("IR provenance hash does not match analyzed PHP source")
	}
	var trigger IRTrigger
	var operation IROperation
	switch {
	case analysis.Platform == "woocommerce" &&
		module == WooCommerceSortingModuleName &&
		analysis.PluginName == "Extra Product Sorting Options for WooCommerce" &&
		analysis.Version == WooCommerceSortingModuleVersion &&
		analysis.SourceSHA256 == WooCommerceSortingSourceSHA256 &&
		provenance.ModuleVersion == WooCommerceSortingModuleVersion &&
		provenance.ArchiveSHA256 == wooCommerceSortingArchiveSHA256 &&
		provenance.SourceReference == WooCommerceSortingSourceReference &&
		provenance.AdapterVersion == wooCommerceAdapterVersion &&
		containsPHPHook(analysis.Hooks, "woocommerce_get_catalog_ordering_args"):
		trigger = IRTrigger{
			Kind: "hook",
			Name: "woocommerce_get_catalog_ordering_args",
			Fields: map[string]string{
				"locale":  "string",
				"orderby": "string",
			},
		}
		operation = IROperation{
			Call: "catalog.product.sort",
			Input: map[string]IRValue{
				"locale":  {TriggerField: "locale"},
				"orderby": {TriggerField: "orderby"},
			},
		}
	case analysis.Platform == "woocommerce" &&
		len(analysis.Hooks) == 1 &&
		analysis.Hooks[0] == "woocommerce_product_query":
		trigger = IRTrigger{Kind: "hook", Name: analysis.Hooks[0]}
		operation = IROperation{Call: "catalog.product.list", Input: map[string]IRValue{}}
	case analysis.Platform == "prestashop" &&
		module == "ps_newproducts" &&
		len(analysis.Hooks) == 1 &&
		analysis.Hooks[0] == "displayHome" &&
		hasPHPHookDetail(analysis.HookDetails, PHPHook{Name: "displayHome", Kind: "display", Handler: "renderWidget"}) &&
		hasPHPFeature(analysis.Features, "prestashop.catalog.new_products"):
		trigger = IRTrigger{Kind: "hook", Name: "displayHome"}
		operation = IROperation{Call: "catalog.product.newest", Input: map[string]IRValue{}}
	default:
		return ModuleIR{}, errors.New("no IR lowering rule for PHP analysis")
	}
	ir := ModuleIR{
		Version:    IRVersion,
		Platform:   analysis.Platform,
		Module:     module,
		Provenance: provenance,
		Triggers:   []IRTrigger{trigger},
		Operations: []IROperation{operation},
	}
	if _, err := ValidateIR(ir); err != nil {
		return ModuleIR{}, err
	}
	return ir, nil
}

func containsPHPHook(hooks []string, name string) bool {
	for _, hook := range hooks {
		if hook == name {
			return true
		}
	}
	return false
}

func ValidateIR(ir ModuleIR) (*ValidatedIR, error) {
	if ir.Version != IRVersion {
		return nil, fmt.Errorf("unsupported IR version %q", ir.Version)
	}
	switch ir.Platform {
	case "woocommerce", "prestashop", "shopify", "magento":
	default:
		return nil, fmt.Errorf("unsupported IR platform %q", ir.Platform)
	}
	if err := ir.Provenance.Validate(ir.Platform); err != nil {
		return nil, fmt.Errorf("invalid IR provenance: %w", err)
	}
	if strings.TrimSpace(ir.Module) == "" || strings.TrimSpace(ir.Module) != ir.Module {
		return nil, errors.New("IR module name is required and must be trimmed")
	}
	if len(ir.Triggers) != 1 {
		return nil, errors.New("IR v1 requires exactly one trigger")
	}
	if strings.TrimSpace(ir.Triggers[0].Kind) == "" || strings.TrimSpace(ir.Triggers[0].Name) == "" {
		return nil, errors.New("IR trigger kind and name are required")
	}
	for field, fieldType := range ir.Triggers[0].Fields {
		if !validIRFieldName(field) || (fieldType != "string" && fieldType != "uuid" && fieldType != "integer") {
			return nil, fmt.Errorf("invalid IR trigger field %q type %q", field, fieldType)
		}
	}
	if len(ir.Operations) != 1 {
		return nil, errors.New("IR v1 requires exactly one operation")
	}

	operation := ir.Operations[0]
	call, exists := wasm.Lookup(operation.Call)
	if !exists {
		return nil, fmt.Errorf("IR operation references unknown host call %q", operation.Call)
	}
	if err := validateIROperationInput(operation, ir.Triggers[0]); err != nil {
		return nil, err
	}
	return &ValidatedIR{
		ModuleIR:     ir,
		Capabilities: []string{string(call.Cap)},
	}, nil
}

func validateIROperationInput(operation IROperation, trigger IRTrigger) error {
	schema, exists := operationInputSchema(operation.Call)
	if !exists {
		return fmt.Errorf("IR operation schema is not implemented for host call %q", operation.Call)
	}
	if len(operation.Input) != len(schema) {
		return fmt.Errorf("invalid IR operation input fields for %q", operation.Call)
	}
	for field, fieldType := range schema {
		value, exists := operation.Input[field]
		if !exists {
			return fmt.Errorf("missing IR operation input field %q", field)
		}
		if (len(value.Literal) == 0) == (value.TriggerField == "") {
			return fmt.Errorf("IR input field %q must be exactly one literal or trigger reference", field)
		}
		if value.TriggerField != "" {
			if !validIRFieldName(value.TriggerField) || trigger.Fields[value.TriggerField] != fieldType {
				return fmt.Errorf("IR input field %q references an undeclared or incompatible trigger field %q", field, value.TriggerField)
			}
			continue
		}
		if err := validateIRLiteral(value.Literal, fieldType); err != nil {
			return fmt.Errorf("invalid IR operation input field %q", field)
		}
	}
	return nil
}

func operationInputSchema(call string) (map[string]string, bool) {
	switch call {
	case "catalog.product.list", "catalog.category.list":
		return map[string]string{}, true
	case "catalog.product.get":
		return map[string]string{"product_id": "uuid"}, true
	case "catalog.supplier.product.preview":
		return map[string]string{"product_json": "string"}, true
	case "catalog.product.newest":
		return map[string]string{}, true
	case "catalog.product.sort":
		return map[string]string{"locale": "string", "orderby": "string"}, true
	default:
		return nil, false
	}
}

func validateIRLiteral(literal json.RawMessage, fieldType string) error {
	if fieldType == "integer" {
		var value int
		if err := json.Unmarshal(literal, &value); err != nil {
			return err
		}
		if value < 0 {
			return errors.New("integer IR values must be non-negative")
		}
		return nil
	}
	var value string
	if err := json.Unmarshal(literal, &value); err != nil {
		return err
	}
	switch fieldType {
	case "string":
		return nil
	case "uuid":
		_, err := abi.RequireUUID("value", value)
		return err
	default:
		return fmt.Errorf("unsupported IR field type %q", fieldType)
	}
}

func validIRFieldName(name string) bool {
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for _, character := range name[1:] {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func BuildIRCallEnvelope(ir ModuleIR, triggerPayload []byte) ([]byte, error) {
	if _, err := ValidateIR(ir); err != nil {
		return nil, err
	}
	if len(triggerPayload) == 0 {
		triggerPayload = []byte(`{}`)
	}
	triggerPayload = bytes.TrimSpace(triggerPayload)
	if len(triggerPayload) == 0 || len(triggerPayload) > abi.MaxCallBytes || triggerPayload[0] != '{' {
		return nil, errors.New("trigger payload must be a bounded JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(triggerPayload))
	triggerValues := map[string]json.RawMessage{}
	if err := decoder.Decode(&triggerValues); err != nil || triggerValues == nil {
		return nil, errors.New("invalid trigger payload")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errors.New("invalid trigger payload")
	}
	trigger := ir.Triggers[0]
	if len(triggerValues) != len(trigger.Fields) {
		return nil, errors.New("trigger payload fields do not match the IR schema")
	}
	for field, fieldType := range trigger.Fields {
		raw, exists := triggerValues[field]
		if !exists {
			return nil, fmt.Errorf("missing trigger payload field %q", field)
		}
		if fieldType == "uuid" {
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return nil, fmt.Errorf("invalid trigger payload field %q", field)
			}
			normalized, err := abi.RequireUUID(field, value)
			if err != nil {
				return nil, fmt.Errorf("invalid trigger payload field %q", field)
			}
			triggerValues[field], _ = json.Marshal(normalized)
		} else if err := validateIRLiteral(raw, fieldType); err != nil {
			return nil, fmt.Errorf("invalid trigger payload field %q", field)
		}
	}

	resolvedInput := make(map[string]json.RawMessage, len(ir.Operations[0].Input))
	for field, value := range ir.Operations[0].Input {
		if value.TriggerField != "" {
			resolvedInput[field] = triggerValues[value.TriggerField]
		} else {
			resolvedInput[field] = value.Literal
		}
	}
	input, err := json.Marshal(resolvedInput)
	if err != nil {
		return nil, fmt.Errorf("encode resolved IR input: %w", err)
	}
	envelope, err := json.Marshal(struct {
		ABI   string          `json:"abi"`
		Call  string          `json:"call"`
		Input json.RawMessage `json:"input"`
	}{ABI: abi.Version, Call: ir.Operations[0].Call, Input: input})
	if err != nil {
		return nil, fmt.Errorf("encode ABI envelope: %w", err)
	}
	if len(envelope) > abi.MaxCallBytes {
		return nil, errors.New("IR ABI envelope exceeds the call size limit")
	}
	return envelope, nil
}

func CompileIRToWAT(ir ModuleIR) (string, error) {
	if _, err := ValidateIR(ir); err != nil {
		return "", err
	}
	if operationHasDynamicInput(ir.Operations[0]) {
		return fmt.Sprintf(`(module
  (import "euromni" "call" (func $call (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") %d)
  (func (export "run") (param $input_ptr i32) (param $input_len i32) (result i32)
    (call $call (local.get $input_ptr) (local.get $input_len) (i32.const %d) (i32.const %d)))
)`, irMemoryPages, irOutputOffset, irOutputCapacity), nil
	}
	envelope, err := BuildIRCallEnvelope(ir, nil)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`(module
  (import "euromni" "call" (func $call (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") %d)
  (data (i32.const 0) "%s")
  (func (export "run") (result i32)
    (call $call (i32.const 0) (i32.const %d) (i32.const %d) (i32.const %d)))
)`, irMemoryPages, watString(envelope), len(envelope), irOutputOffset, irOutputCapacity), nil
}

func CompileIRToWasm(ir ModuleIR) ([]byte, error) {
	wat, err := CompileIRToWAT(ir)
	if err != nil {
		return nil, err
	}
	wasmBytes, err := wasmtime.Wat2Wasm(wat)
	if err != nil {
		return nil, fmt.Errorf("compile IR to Wasm: %w", err)
	}
	irBytes, err := json.Marshal(ir)
	if err != nil {
		return nil, fmt.Errorf("encode IR for Wasm provenance section: %w", err)
	}
	irHash := sha256.Sum256(irBytes)
	wasmBytes = appendWASMCustomSection(wasmBytes, wasmIRHashSection, []byte(hex.EncodeToString(irHash[:])))
	return wasmBytes, nil
}

func appendWASMCustomSection(wasmBytes []byte, name string, contents []byte) []byte {
	nameBytes := []byte(name)
	payload := binary.AppendUvarint(nil, uint64(len(nameBytes)))
	payload = append(payload, nameBytes...)
	payload = append(payload, contents...)
	wasmBytes = append(wasmBytes, 0)
	wasmBytes = binary.AppendUvarint(wasmBytes, uint64(len(payload)))
	return append(wasmBytes, payload...)
}

func operationHasDynamicInput(operation IROperation) bool {
	for _, value := range operation.Input {
		if value.TriggerField != "" {
			return true
		}
	}
	return false
}

func watString(value []byte) string {
	var encoded strings.Builder
	for _, character := range value {
		switch {
		case character == '\\':
			encoded.WriteString(`\\`)
		case character == '"':
			encoded.WriteString(`\"`)
		case character >= 0x20 && character <= 0x7e:
			encoded.WriteByte(character)
		default:
			fmt.Fprintf(&encoded, `\%02x`, character)
		}
	}
	return encoded.String()
}
