#![no_std]

extern crate alloc;

use alloc::boxed::Box;
use alloc::string::{String, ToString};
use alloc::vec::Vec;
use core::panic::PanicInfo;
use serde::{Deserialize, Serialize};
use serde_json::value::RawValue;

#[global_allocator]
static ALLOCATOR: wee_alloc::WeeAlloc = wee_alloc::WeeAlloc::INIT;

const ABI: &str = "euromni.abi.v1";
const PREVIEW_RESULT_CALL: &str = "catalog.supplier.product.preview.result";
const MAX_PRODUCT_BYTES: usize = 8 << 10;

#[link(wasm_import_module = "euromni")]
unsafe extern "C" {
    #[link_name = "call"]
    fn euromni_call(input_ptr: i32, input_len: i32, output_ptr: i32, output_capacity: i32) -> i32;
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct PreviewEnvelope {
    abi: String,
    call: String,
    #[serde(rename = "id", default)]
    _id: Option<String>,
    input: PreviewInput,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct PreviewInput {
    product_json: String,
}

#[derive(Deserialize)]
struct SelfnamedProduct {
    id: Box<RawValue>,
    name: String,
    #[serde(default)]
    description: Option<String>,
    price: Box<RawValue>,
    volume: i64,
    weight: i64,
    #[serde(rename = "isInStock")]
    is_in_stock: Option<bool>,
    #[serde(default)]
    tags: Option<Vec<String>>,
}

#[derive(Serialize)]
struct HostEnvelope<'a> {
    abi: &'static str,
    call: &'static str,
    input: HostInput<'a>,
}

#[derive(Serialize)]
struct HostInput<'a> {
    preview: &'a ProductPreview,
}

#[derive(Serialize)]
struct ProductPreview {
    provider: &'static str,
    external_id: String,
    publication_status: &'static str,
    name: String,
    description: String,
    locale: Option<String>,
    price: PricePreview,
    inventory: InventoryPreview,
    weight: WeightPreview,
    volume: VolumePreview,
    tags: Vec<String>,
    unresolved: [&'static str; 3],
}

#[derive(Serialize)]
struct PricePreview {
    amount: String,
    currency: Option<String>,
}

#[derive(Serialize)]
struct InventoryPreview {
    status: &'static str,
    track_quantity: bool,
}

#[derive(Serialize)]
struct WeightPreview {
    grams: i64,
    unit: Option<String>,
}

#[derive(Serialize)]
struct VolumePreview {
    value: i64,
    unit: Option<String>,
}

fn positive_integer(raw: &str) -> bool {
    !raw.is_empty()
        && raw.bytes().all(|byte| byte.is_ascii_digit())
        && raw.bytes().any(|byte| byte != b'0')
}

fn normalize_decimal(raw: &str) -> Option<String> {
    let (whole, fraction) = match raw.split_once('.') {
        Some((whole, fraction)) => (whole, Some(fraction)),
        None => (raw, None),
    };
    if whole.is_empty() || whole.len() > 12 || !whole.bytes().all(|byte| byte.is_ascii_digit()) {
        return None;
    }
    let Some(fraction) = fraction else {
        return Some(raw.to_string());
    };
    if fraction.is_empty()
        || fraction.len() > 4
        || !fraction.bytes().all(|byte| byte.is_ascii_digit())
    {
        return None;
    }
    let trimmed = fraction.trim_end_matches('0');
    if trimmed.is_empty() {
        Some(whole.to_string())
    } else {
        let mut normalized = String::with_capacity(whole.len() + trimmed.len() + 1);
        normalized.push_str(whole);
        normalized.push('.');
        normalized.push_str(trimmed);
        Some(normalized)
    }
}

fn map_product(product_json: &str) -> Option<ProductPreview> {
    if product_json.is_empty() || product_json.len() > MAX_PRODUCT_BYTES {
        return None;
    }
    let product: SelfnamedProduct = serde_json::from_str(product_json).ok()?;
    let external_id = product.id.get();
    if !positive_integer(external_id) || external_id.len() > 20 {
        return None;
    }
    let name = product.name.trim();
    let description = product.description.unwrap_or_default();
    if name.is_empty() || name.len() > 512 || description.len() > 8192 {
        return None;
    }
    let price = normalize_decimal(product.price.get())?;
    if !(0..=100_000_000).contains(&product.weight)
        || !(0..=1_000_000_000).contains(&product.volume)
    {
        return None;
    }
    let is_in_stock = product.is_in_stock?;
    let source_tags = product.tags.unwrap_or_default();
    if source_tags.len() > 64 {
        return None;
    }
    let mut tags = Vec::with_capacity(source_tags.len());
    for tag in source_tags {
        let tag = tag.trim();
        if tag.is_empty() || tag.len() > 128 {
            return None;
        }
        tags.push(tag.to_string());
    }
    Some(ProductPreview {
        provider: "selfnamed",
        external_id: external_id.to_string(),
        publication_status: "draft",
        name: name.to_string(),
        description,
        locale: None,
        price: PricePreview {
            amount: price,
            currency: None,
        },
        inventory: InventoryPreview {
            status: if is_in_stock { "instock" } else { "outofstock" },
            track_quantity: false,
        },
        weight: WeightPreview {
            grams: product.weight,
            unit: None,
        },
        volume: VolumePreview {
            value: product.volume,
            unit: None,
        },
        tags,
        unresolved: ["currency", "locale", "volume_unit"],
    })
}

fn output_request(input: &[u8]) -> Vec<u8> {
    let mapped = serde_json::from_slice::<PreviewEnvelope>(input)
        .ok()
        .filter(|envelope| {
            envelope.abi == ABI && envelope.call == "catalog.supplier.product.preview"
        })
        .and_then(|envelope| map_product(&envelope.input.product_json));
    match mapped {
        Some(preview) => serde_json::to_vec(&HostEnvelope {
            abi: ABI,
            call: PREVIEW_RESULT_CALL,
            input: HostInput { preview: &preview },
        })
        .unwrap_or_else(|_| invalid_request()),
        None => invalid_request(),
    }
}

fn invalid_request() -> Vec<u8> {
    b"{\"abi\":\"euromni.abi.v1\",\"call\":\"catalog.supplier.product.preview.result\",\"input\":{\"preview\":{}}}".to_vec()
}

#[no_mangle]
pub extern "C" fn run(input_ptr: i32, input_len: i32) -> i32 {
    if input_ptr < 0 || input_len <= 0 {
        return -1;
    }
    let input = unsafe { core::slice::from_raw_parts(input_ptr as *const u8, input_len as usize) };
    let request = output_request(input);
    let mut response: Vec<u8> = Vec::with_capacity(16 << 10);
    unsafe {
        euromni_call(
            request.as_ptr() as i32,
            request.len() as i32,
            response.as_mut_ptr() as i32,
            response.capacity() as i32,
        )
    }
}

#[panic_handler]
fn panic(_info: &PanicInfo) -> ! {
    loop {}
}
