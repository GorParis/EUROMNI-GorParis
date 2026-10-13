package main

import (
	"runtime"
	"unsafe"
)

//go:wasmimport euromni call
func euromniCall(inputPtr unsafe.Pointer, inputLen uint32, outputPtr unsafe.Pointer, outputCapacity uint32) int32

//go:wasmexport run
func run() int32 {
	request := []byte(`{"abi":"euromni.abi.v1","call":"catalog.product.list","input":{}}`)
	response := make([]byte, 256)
	written := euromniCall(unsafe.Pointer(&request[0]), uint32(len(request)), unsafe.Pointer(&response[0]), uint32(len(response)))
	runtime.KeepAlive(request)
	runtime.KeepAlive(response)
	if written <= 0 {
		return written
	}
	return int32(response[0])
}

func main() {}