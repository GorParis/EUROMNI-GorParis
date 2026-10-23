<?php

function pilot_assert( $condition, string $message ): void {
	if ( ! $condition ) {
		WP_CLI::error( $message );
	}
}

add_filter( 'woocommerce_email_enabled_new_order', '__return_false' );
add_filter( 'woocommerce_email_enabled_customer_processing_order', '__return_false' );

update_option( 'selfnamed_api_url', 'http://mock-api:8080' );
update_option( 'selfnamed_api_key', 'EUROMNI_LOCAL_FAKE_TOKEN_NOT_A_CREDENTIAL' );
update_option( 'selfnamed_order_sync_mode', 'manual' );

$product_importer = new Selfnamed\Model\Product();
pilot_assert( $product_importer->create_all_products( 1, 10 ), 'Selfnamed mock catalog import failed.' );
pilot_assert_catalog_product( 842002, 'Synthetic Catalog Balm', '28.82', '0.176', '176', 'instock' );
pilot_assert_catalog_product( 842003, 'Synthetic Out-of-Stock Balm', '17.4', '1.001', '88', 'outofstock' );
pilot_assert_catalog_product( 842004, 'Synthetic Decimal-Price Balm', '0.1', '1', '1', 'instock' );

$tagged_product = get_posts(
	array(
		'post_type'   => 'product',
		'post_status' => 'draft',
		'numberposts' => 1,
		'meta_key'    => '_selfnamed_product_id',
		'meta_value'  => 842003,
	)
);
pilot_assert(
	1 === count( $tagged_product ) &&
	array( 'edge-case', 'synthetic' ) === wp_get_object_terms( $tagged_product[0]->ID, 'product_tag', array( 'fields' => 'slugs' ) ),
	'Imported product tags were not persisted.'
);

pilot_assert( $product_importer->create_all_products( 1, 10 ), 'Repeated Selfnamed mock catalog import failed.' );
foreach ( array( 842002, 842003, 842004 ) as $selfnamed_id ) {
	$reimported_products = wc_get_products(
		array(
			'limit'        => -1,
			'meta_key'     => '_selfnamed_product_id',
			'meta_value'   => $selfnamed_id,
			'meta_compare' => '=',
		)
	);
	pilot_assert( 1 === count( $reimported_products ), sprintf( 'Repeated import created a duplicate for product %d.', $selfnamed_id ) );
}

update_option( 'selfnamed_api_url', 'http://mock-api:8080/unauthorized' );
pilot_assert( false === $product_importer->create_all_products( 1, 10 ), 'Unauthorized catalog API was reported as a successful import.' );
update_option( 'selfnamed_api_url', 'http://mock-api:8080' );

update_option(
	'_selfnamed_billing_information',
	serialize(
		array(
			'name'     => 'EUROMNI Synthetic Merchant',
			'country'  => 'FR',
			'city'     => 'Testville',
			'street'   => '1 Rue Synthetic',
			'streetNo' => '1',
			'flatNo'   => '',
			'zip'      => '00000',
			'vatNr'    => '',
			'regNr'    => '',
			'phone'    => '+33000000000',
			'email'    => 'merchant@example.invalid',
		)
	)
);

$product = new WC_Product_Simple();
$product->set_name( 'Synthetic Selfnamed Serum' );
$product->set_status( 'publish' );
$product->set_regular_price( '12.50' );
$product->set_sku( 'EUROMNI-SELFNAMED-PILOT' );
$product->update_meta_data( '_selfnamed_product_id', 842001 );
$product->save();

$order = wc_create_order();
$order->add_product( $product, 2 );
$order->set_billing_first_name( 'Synthetic' );
$order->set_billing_last_name( 'Buyer' );
$order->set_billing_email( 'buyer@example.invalid' );
$order->set_billing_phone( '+33000000000' );
$order->set_billing_address_1( '1 Rue Synthetic' );
$order->set_billing_city( 'Testville' );
$order->set_billing_postcode( '00000' );
$order->set_billing_country( 'FR' );
$order->set_shipping_first_name( 'Synthetic' );
$order->set_shipping_last_name( 'Buyer' );
$order->set_shipping_phone( '+33000000000' );
$order->set_shipping_address_1( '1 Rue Synthetic' );
$order->set_shipping_city( 'Testville' );
$order->set_shipping_postcode( '00000' );
$order->set_shipping_country( 'FR' );
$order->set_currency( 'EUR' );
$order->set_payment_method( 'local-pilot-no-payment' );
$order->set_payment_method_title( 'Local pilot only; no payment' );

$shipping = new WC_Order_Item_Shipping();
$shipping->set_method_title( 'Synthetic Selfnamed delivery' );
$shipping->set_method_id( 'selfnamed_shipping' );
$shipping->set_total( '3.00' );
$shipping->add_meta_data( '_selfnamed_method_shipping_group', 'pilot-eu-standard', true );
$order->add_item( $shipping );
$order->calculate_totals( false );
$order->save();
$order->update_status( 'processing' );

$sync = new Selfnamed\Model\Orders_Sync();
$sync->sync( array( $order->get_id() ) );
$sync->sync( array( $order->get_id() ) );

$synced_order = wc_get_order( $order->get_id() );
pilot_assert( '9001' === (string) $synced_order->get_meta( '_selfnamed_order_id' ), 'Selfnamed mock order ID was not saved.' );
pilot_assert( 'PROCESSING' === $synced_order->get_meta( '_selfnamed_order_status' ), 'Selfnamed mock order status was not saved.' );

$response = wp_remote_get( 'http://mock-api:8080/assertions', array( 'timeout' => 5 ) );
pilot_assert( ! is_wp_error( $response ), 'Could not read assertions from the local API mock.' );
pilot_assert( 200 === wp_remote_retrieve_response_code( $response ), 'The local API mock returned an unexpected status.' );

$assertions = json_decode( wp_remote_retrieve_body( $response ), true );
pilot_assert( is_array( $assertions ), 'The local API mock returned invalid assertions.' );
pilot_assert(
	1 === $assertions['product_lookup_ids']['842001'],
	sprintf(
		'Expected one lookup for the synthetic order product; got %d.',
		$assertions['product_lookup_ids']['842001'] ?? 0
	)
);
pilot_assert( 2 === $assertions['catalog_product_lookups'], 'Expected the initial import and re-import catalog requests.' );
pilot_assert( 1 === $assertions['catalog_error_lookups'], 'Expected one simulated unauthorized catalog request.' );
pilot_assert( 1 === $assertions['order_posts'], 'Expected exactly one order submission despite the repeated sync.' );
pilot_assert( true === $assertions['payload_valid'], 'The synthetic buyer, product, or shipping data did not match the expected payload.' );

WP_CLI::success( 'Selfnamed 2.0.19 fixture matrix, failure path, idempotent re-import, and isolated order flow passed; fake credential only.' );

function pilot_assert_catalog_product( int $selfnamed_id, string $name, string $price, string $weight, string $volume, string $stock_status ): void {
	$products = wc_get_products(
		array(
			'limit'        => -1,
			'meta_key'     => '_selfnamed_product_id',
			'meta_value'   => $selfnamed_id,
			'meta_compare' => '=',
		)
	);
	pilot_assert( 1 === count( $products ), sprintf( 'Expected one WooCommerce product for Selfnamed ID %d.', $selfnamed_id ) );
	$product = $products[0];
	$post    = get_post( $product->get_id() );
	pilot_assert( 'draft' === $post->post_status, sprintf( 'Product %d was not kept in draft.', $selfnamed_id ) );
	pilot_assert( $name === $product->get_name(), sprintf( 'Product %d name mismatch.', $selfnamed_id ) );
	pilot_assert(
		$price === $product->get_regular_price(),
		sprintf( 'Product %d regular price = %s, want %s.', $selfnamed_id, $product->get_regular_price(), $price )
	);
	pilot_assert(
		$weight === $product->get_weight(),
		sprintf( 'Product %d weight = %s, want %s.', $selfnamed_id, $product->get_weight(), $weight )
	);
	pilot_assert( $stock_status === $product->get_stock_status(), sprintf( 'Product %d stock status mismatch.', $selfnamed_id ) );
	pilot_assert( false === $product->get_manage_stock(), sprintf( 'Product %d unexpectedly manages stock quantity.', $selfnamed_id ) );
	pilot_assert( $volume === get_post_meta( $product->get_id(), '_volume', true ), sprintf( 'Product %d volume metadata mismatch.', $selfnamed_id ) );
}
