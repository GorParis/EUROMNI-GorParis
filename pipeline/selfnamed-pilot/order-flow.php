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
$imported_products = wc_get_products(
	array(
		'limit'        => -1,
		'meta_key'     => '_selfnamed_product_id',
		'meta_value'   => 842002,
		'meta_compare' => '=',
	)
);
pilot_assert( 1 === count( $imported_products ), 'Expected exactly one imported synthetic product.' );
$imported_product = $imported_products[0];
$imported_post    = get_post( $imported_product->get_id() );
pilot_assert( 'draft' === $imported_post->post_status, 'Imported product was not kept in draft.' );
pilot_assert( 'Synthetic Catalog Balm' === $imported_product->get_name(), 'Imported product name did not match.' );
pilot_assert( 'Synthetic description used to characterize product mapping.' === $imported_product->get_description(), 'Imported product description did not match.' );
pilot_assert( '28.82' === $imported_product->get_regular_price(), 'Imported regular price did not match.' );
pilot_assert( '0.176' === $imported_product->get_weight(), 'Imported weight was not converted from grams to kilograms.' );
pilot_assert( 'instock' === $imported_product->get_stock_status(), 'Imported stock status did not match.' );
pilot_assert( false === $imported_product->get_manage_stock(), 'Imported product unexpectedly manages stock quantity.' );
pilot_assert( '176' === get_post_meta( $imported_product->get_id(), '_volume', true ), 'Imported volume metadata did not match.' );

pilot_assert( $product_importer->create_all_products( 1, 10 ), 'Repeated Selfnamed mock catalog import failed.' );
$reimported_products = wc_get_products(
	array(
		'limit'        => -1,
		'meta_key'     => '_selfnamed_product_id',
		'meta_value'   => 842002,
		'meta_compare' => '=',
	)
);
pilot_assert( 1 === count( $reimported_products ), 'Repeated catalog import created a duplicate product.' );

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
pilot_assert(
	2 === $assertions['catalog_product_lookups'],
	sprintf( 'Expected two catalog lookups; got %d.', $assertions['catalog_product_lookups'] )
);
pilot_assert( 1 === $assertions['order_posts'], 'Expected exactly one order submission despite the repeated sync.' );
pilot_assert( true === $assertions['payload_valid'], 'The synthetic buyer, product, or shipping data did not match the expected payload.' );

WP_CLI::success( 'Isolated Selfnamed catalog mapping, order, and shipping flows passed; fake credential only, no external API.' );
