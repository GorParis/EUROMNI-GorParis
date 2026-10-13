(module
  (import "euromni" "call" (func $call (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") 2)
  (data (i32.const 0) "{\"abi\":\"euromni.abi.v1\",\"call\":\"catalog.product.get\",\"input\":{\"product_id\":\"e0e00000-0001-4000-8000-000000000040\"}}")
  (func (export "run") (result i32)
    (drop
      (call $call
        (i32.const 0)
        (i32.const 128)
        (i32.const 32768)
        (i32.const 32768)))
    (i32.load8_u (i32.const 32768)))
)
