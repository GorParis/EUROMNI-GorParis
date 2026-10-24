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

pilot_assert_shipping_behavior();

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

update_option( 'selfnamed_platform_access_token', 'EUROMNI_LOCAL_FAKE_CALLBACK_TOKEN' );
$unauthorized_notification = pilot_send_notification(
	array(
		'type'        => 'PRODUCT',
		'relatedId'   => 842002,
		'accessToken' => 'wrong-local-token',
	)
);
pilot_assert(
	401 === wp_remote_retrieve_response_code( $unauthorized_notification ),
	'Notification endpoint accepted an invalid callback token.'
);

$product_notification = pilot_send_notification(
	array(
		'type'        => 'PRODUCT',
		'relatedId'   => 842002,
		'accessToken' => 'EUROMNI_LOCAL_FAKE_CALLBACK_TOKEN',
	)
);
pilot_assert( 200 === wp_remote_retrieve_response_code( $product_notification ), 'Synthetic product notification failed.' );
$callback_products = wc_get_products(
	array(
		'limit'        => -1,
		'meta_key'     => '_selfnamed_product_id',
		'meta_value'   => 842002,
		'meta_compare' => '=',
	)
);
pilot_assert( 1 === count( $callback_products ), 'Expected one product for the callback target.' );
$callback_product_id = $callback_products[0]->get_id();
clean_post_cache( $callback_product_id );
$updated_product = wc_get_product( $callback_product_id );
pilot_assert( 'Synthetic Callback Balm' === $updated_product->get_name(), 'Product callback did not update the title.' );
pilot_assert( '31.25' === $updated_product->get_regular_price(), 'Product callback did not update the price.' );
pilot_assert( '0.25' === $updated_product->get_weight(), 'Product callback did not update grams-to-kilograms weight.' );
pilot_assert( 'outofstock' === $updated_product->get_stock_status(), 'Product callback did not update the stock status.' );
pilot_assert( '210' === get_post_meta( $updated_product->get_id(), '_volume', true ), 'Product callback did not update volume.' );

$order_notification = pilot_send_notification(
	array(
		'type'        => 'ORDER',
		'relatedId'   => 9001,
		'accessToken' => 'EUROMNI_LOCAL_FAKE_CALLBACK_TOKEN',
	)
);
pilot_assert( 200 === wp_remote_retrieve_response_code( $order_notification ), 'Synthetic shipped-order notification failed.' );
clean_post_cache( $order->get_id() );
$synced_order = wc_get_order( $order->get_id() );
$synced_order->read_meta_data( true );
pilot_assert( 'completed' === $synced_order->get_status(), 'SHIPPED callback did not complete the WooCommerce order.' );
pilot_assert(
	'SHIPPED' === $synced_order->get_meta( '_selfnamed_order_status' ),
	sprintf( 'SHIPPED callback status was not saved (actual: %s).', $synced_order->get_meta( '_selfnamed_order_status' ) )
);
pilot_assert(
	'PILOT-TRACK-9001, PILOT-TRACK-9001-B' === $synced_order->get_meta( 'selfnamed_order_tracking_numbers' ),
	'SHIPPED callback tracking numbers were not saved.'
);
pilot_assert(
	'https://tracking.example.invalid/9001, https://tracking.example.invalid/9001-b' === $synced_order->get_meta( 'selfnamed_order_tracking_urls' ),
	'SHIPPED callback tracking URLs were not saved.'
);

$cancelled_order = wc_create_order();
$cancelled_order->set_status( 'processing' );
$cancelled_order->update_meta_data( '_selfnamed_order_id', 9002 );
$cancelled_order->save();
$cancelled_notification = pilot_send_notification(
	array(
		'type'        => 'ORDER',
		'relatedId'   => 9002,
		'accessToken' => 'EUROMNI_LOCAL_FAKE_CALLBACK_TOKEN',
	)
);
pilot_assert( 200 === wp_remote_retrieve_response_code( $cancelled_notification ), 'Synthetic cancellation notification failed.' );
clean_post_cache( $cancelled_order->get_id() );
$cancelled_order = wc_get_order( $cancelled_order->get_id() );
$cancelled_order->read_meta_data( true );
pilot_assert( 'cancelled' === $cancelled_order->get_status(), 'CANCELLED callback did not cancel the WooCommerce order.' );
pilot_assert( 'Synthetic cancellation' === $cancelled_order->get_meta( '_selfnamed_order_message' ), 'Cancellation reason was not saved.' );

$store_notification = pilot_send_notification(
	array(
		'type'        => 'STORE',
		'accessToken' => 'EUROMNI_LOCAL_FAKE_CALLBACK_TOKEN',
	)
);
pilot_assert( 200 === wp_remote_retrieve_response_code( $store_notification ), 'Synthetic store notification failed.' );
pilot_assert(
	true === json_decode( get_option( 'selfnamed_is_using_custom_shipping_profile' ), true ),
	'STORE callback did not refresh the shipping-profile option.'
);

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
	1 === $assertions['product_lookup_ids']['842002'],
	'Expected exactly one supplier lookup for the valid product callback.'
);
pilot_assert( 2 === $assertions['catalog_product_lookups'], 'Expected the initial import and re-import catalog requests.' );
pilot_assert( 1 === $assertions['catalog_error_lookups'], 'Expected one simulated unauthorized catalog request.' );
pilot_assert( 2 === $assertions['shipping_zone_lookups'], 'Expected one cached shipping-zone lookup and one direct mock contract check.' );
pilot_assert( 1 === $assertions['store_lookups'], 'Expected one store-profile refresh lookup.' );
pilot_assert(
	array( '9001' => 1, '9002' => 1 ) === $assertions['order_lookups'],
	'Expected exactly one API lookup for each synthetic order notification.'
);
pilot_assert( 1 === $assertions['order_posts'], 'Expected exactly one order submission despite the repeated sync.' );
pilot_assert( true === $assertions['payload_valid'], 'The synthetic buyer, product, or shipping data did not match the expected payload.' );

WP_CLI::success( 'Selfnamed 2.0.19 catalog, shipping, order, and callback mock flows passed; fake credentials only.' );

function pilot_send_notification( array $payload ) {
	$response = wp_remote_post(
		'http://wordpress/wp-json/sn/notification',
		array(
			'timeout' => 5,
			'headers' => array( 'Content-Type' => 'application/json' ),
			'body'    => wp_json_encode( $payload ),
		)
	);
	pilot_assert( ! is_wp_error( $response ), 'Could not call the local WordPress notification endpoint.' );

	return $response;
}

function pilot_assert_shipping_behavior(): void {
	update_option( 'woocommerce_selfnamed_shipping_settings', array( 'enabled' => 'yes' ) );
	$shipping_method = new Selfnamed\Model\Shipping_Method();
	$shipping_method->enabled = 'yes';
	pilot_assert( $shipping_method->is_enabled(), 'Selfnamed shipping method did not enable for the controlled test.' );

	$existing_products = wc_get_products(
		array(
			'limit' => 1,
			'sku'   => 'EUROMNI-SELFNAMED-PILOT-SHIPPING',
		)
	);
	$selfnamed_product = $existing_products ? $existing_products[0] : false;
	if ( ! $selfnamed_product ) {
		$selfnamed_product = new WC_Product_Simple();
		$selfnamed_product->set_name( 'Synthetic Shipping Product' );
		$selfnamed_product->set_status( 'publish' );
		$selfnamed_product->set_regular_price( '10.00' );
		$selfnamed_product->set_weight( '1.002' );
		$selfnamed_product->set_sku( 'EUROMNI-SELFNAMED-PILOT-SHIPPING' );
		$selfnamed_product->update_meta_data( '_selfnamed_product_id', 842003 );
		$selfnamed_product->save();
	}

	$regular_product = new WC_Product_Simple();
	$regular_product->set_name( 'Synthetic Non-Selfnamed Product' );
	$regular_product->set_weight( '0.500' );
	$regular_product->set_regular_price( '8.00' );
	$package = array(
		'contents' => array(
			'selfnamed' => array(
				'product_id' => $selfnamed_product->get_id(),
				'data'       => $selfnamed_product,
				'quantity'   => 1,
				'line_total' => 10.00,
			),
			'regular'   => array(
				'product_id' => 0,
				'data'       => $regular_product,
				'quantity'   => 1,
				'line_total' => 8.00,
			),
		),
		'destination' => array( 'country' => 'FR' ),
	);
	$split_packages = $shipping_method->mark_selfnamed_packages( array( $package ) );
	pilot_assert( 2 === count( $split_packages ), 'Mixed cart was not split into supplier and regular shipping packages.' );

	$selfnamed_package = null;
	$regular_package = null;
	foreach ( $split_packages as $split_package ) {
		if ( ! empty( $split_package['selfnamed'] ) ) {
			$selfnamed_package = $split_package;
		} else {
			$regular_package = $split_package;
		}
	}
	pilot_assert( is_array( $selfnamed_package ) && is_array( $regular_package ), 'Mixed-cart package flags are incorrect.' );
	pilot_assert( 1 === count( $selfnamed_package['contents'] ), 'Selfnamed shipping package contains a non-Selfnamed line.' );
	pilot_assert( 1 === count( $regular_package['contents'] ), 'Regular shipping package contains a Selfnamed line.' );

	$shipping_zones = ( new Selfnamed\Model\Api() )->get_all_shipping_zones();
	pilot_assert(
		! $shipping_zones->error &&
		1 === count( $shipping_zones->data ) &&
		array( 'FR', 'BE' ) === $shipping_zones->data[0]->countries,
		'Selfnamed mock shipping zones were not returned with the expected countries.'
	);

	$rates = $shipping_method->get_rates_for_package( $selfnamed_package );
	$rate = $rates['selfnamed_shipping_71'] ?? null;
	pilot_assert(
		$rate instanceof WC_Shipping_Rate,
		sprintf( 'No Selfnamed rate was returned for the synthetic FR package (%s).', wp_json_encode( $rates ) )
	);
	pilot_assert( '7.25' === $rate->get_cost(), '1,002 g package did not select the expected weight-band price.' );
	pilot_assert(
		'PILOT-EU-STANDARD' === $rate->get_meta_data()['_selfnamed_method_shipping_group'],
		'Selfnamed shipping group was not retained on the rate.'
	);

	$existing_products = wc_get_products(
		array(
			'limit' => 1,
			'sku'   => 'EUROMNI-SELFNAMED-PILOT-BOUNDARY',
		)
	);
	$boundary_product = $existing_products ? $existing_products[0] : false;
	if ( ! $boundary_product ) {
		$boundary_product = new WC_Product_Simple();
		$boundary_product->set_name( 'Synthetic One-Kilogram Product' );
		$boundary_product->set_status( 'publish' );
		$boundary_product->set_regular_price( '10.00' );
		$boundary_product->set_weight( '1.000' );
		$boundary_product->set_sku( 'EUROMNI-SELFNAMED-PILOT-BOUNDARY' );
		$boundary_product->update_meta_data( '_selfnamed_product_id', 842004 );
		$boundary_product->save();
	}
	$boundary_package = array(
		'contents' => array(
			'boundary' => array(
				'product_id' => $boundary_product->get_id(),
				'data'       => $boundary_product,
				'quantity'   => 1,
				'line_total' => 10.00,
			),
		),
		'destination' => array( 'country' => 'FR' ),
	);
	$boundary_rates = $shipping_method->get_rates_for_package( $boundary_package );
	$boundary_rate = $boundary_rates['selfnamed_shipping_71'] ?? null;
	pilot_assert( $boundary_rate instanceof WC_Shipping_Rate, 'No Selfnamed rate was returned at the 1,000 g boundary.' );
	pilot_assert( '4.5' === $boundary_rate->get_cost(), '1,000 g package did not match the inclusive lower shipping band.' );

	$unsupported_package = $boundary_package;
	$unsupported_package['destination']['country'] = 'US';
	pilot_assert(
		0 === count( $shipping_method->get_rates_for_package( $unsupported_package ) ),
		'Selfnamed returned a shipping rate for an unsupported destination.'
	);
}

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
