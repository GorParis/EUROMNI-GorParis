(module
  (memory (export "memory") 1)
  (data (i32.const 1024) "\"subtotal_units\":")
  (data (i32.const 1048) "\"base_units\":")
  (func $number_after_key (param $ptr i32) (param $len i32) (param $key i32) (param $key_len i32) (result i64)
    (local $i i32)
    (local $j i32)
    (local $value i64)
    (local $digit i32)
    (block $not_found
      (loop $scan
        (br_if $not_found (i32.gt_u (local.get $i) (i32.sub (local.get $len) (local.get $key_len))))
        (local.set $j (i32.const 0))
        (block $mismatch
          (loop $match
            (br_if $mismatch (i32.ge_u (local.get $j) (local.get $key_len)))
            (br_if $mismatch
              (i32.ne
                (i32.load8_u (i32.add (local.get $ptr) (i32.add (local.get $i) (local.get $j))))
                (i32.load8_u (i32.add (local.get $key) (local.get $j)))))
            (local.set $j (i32.add (local.get $j) (i32.const 1)))
            (br $match)))
        (if (i32.eq (local.get $j) (local.get $key_len))
          (then
            (local.set $i (i32.add (local.get $i) (local.get $key_len)))
            (block $end_digits
              (loop $digits
                (br_if $end_digits (i32.ge_u (local.get $i) (local.get $len)))
                (local.set $digit (i32.load8_u (i32.add (local.get $ptr) (local.get $i))))
                (br_if $end_digits (i32.lt_u (local.get $digit) (i32.const 48)))
                (br_if $end_digits (i32.gt_u (local.get $digit) (i32.const 57)))
                (local.set $value
                  (i64.add
                    (i64.mul (local.get $value) (i64.const 10))
                    (i64.extend_i32_u (i32.sub (local.get $digit) (i32.const 48)))))
                (local.set $i (i32.add (local.get $i) (i32.const 1)))
                (br $digits)))
            (return (local.get $value))))
        (local.set $i (i32.add (local.get $i) (i32.const 1)))
        (br $scan)))
    (i64.const -1))
  (func (export "run") (param $ptr i32) (param $len i32) (result i64)
    (local $subtotal i64)
    (local $base i64)
    (local.set $subtotal
      (call $number_after_key (local.get $ptr) (local.get $len) (i32.const 1024) (i32.const 17)))
    (local.set $base
      (call $number_after_key (local.get $ptr) (local.get $len) (i32.const 1048) (i32.const 13)))
    (if (i64.lt_s (local.get $subtotal) (i64.const 0)) (then (return (i64.const -1))))
    (if (i64.lt_s (local.get $base) (i64.const 0)) (then (return (i64.const -1))))
    (if (i64.ge_u (local.get $subtotal) (i64.const 1000000))
      (then (return (i64.const 0))))
    (local.get $base))
)
