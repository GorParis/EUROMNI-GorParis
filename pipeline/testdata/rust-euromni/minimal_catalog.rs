#![no_std]

use core::panic::PanicInfo;

const OUTPUT_OFFSET: i32 = 16_384;
const OUTPUT_CAPACITY: i32 = 16_384;

#[link(wasm_import_module = "euromni")]
unsafe extern "C" {
    #[link_name = "call"]
    fn euromni_call(input_ptr: i32, input_len: i32, output_ptr: i32, output_capacity: i32) -> i32;
}

#[no_mangle]
pub extern "C" fn run(input_ptr: i32, input_len: i32) -> i32 {
    unsafe { euromni_call(input_ptr, input_len, OUTPUT_OFFSET, OUTPUT_CAPACITY) }
}

#[panic_handler]
fn panic(_info: &PanicInfo) -> ! {
    loop {}
}
