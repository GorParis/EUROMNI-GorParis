const template = document.createElement("template");
template.innerHTML = `
  <style>
    :host {
      display: block;
    }
    .status {
      margin: 1rem 0;
      color: #57534e;
      text-align: center;
    }
    .status[data-kind="error"] {
      color: #9f1239;
    }
    .category-toolbar {
      display: flex;
      flex-wrap: wrap;
      gap: 0.65rem;
      margin: 0 0 1rem;
    }
    .category-button {
      min-height: 2.35rem;
      padding: 0.55rem 0.8rem;
      border: 1px solid #d6cdbd;
      border-radius: 999px;
      background: #f8f5f0;
      color: #292524;
      cursor: pointer;
      font: 600 0.8rem/1.3 system-ui, sans-serif;
    }
    .category-button[aria-pressed="true"] {
      background: #292524;
      border-color: #292524;
      color: #fffefa;
    }
    .product-list {
      list-style: none;
      margin: 0;
      padding: 0;
      display: grid;
      gap: 1.25rem;
      grid-template-columns: repeat(auto-fill, minmax(min(100%, 15rem), 1fr));
    }
    .product-list[hidden],
    .detail-view[hidden],
    .collection-title[hidden],
    .category-toolbar[hidden] {
      display: none;
    }
    article {
      height: 100%;
      background: #fffefa;
      border: 1px solid #e7e2d8;
      border-radius: 0.75rem;
      overflow: hidden;
      transition: transform 160ms ease, box-shadow 160ms ease;
    }
    article:hover {
      transform: translateY(-2px);
      box-shadow: 0 0.75rem 1.75rem rgb(28 25 23 / 9%);
    }
    img {
      display: block;
      width: 100%;
      aspect-ratio: 4 / 3;
      object-fit: cover;
      background: #e7e5e4;
    }
    .placeholder {
      display: grid;
      aspect-ratio: 4 / 3;
      place-items: center;
      background: linear-gradient(145deg, #ebe5d9, #d9d0c0);
      color: #71685c;
      font: 0.8rem/1.4 system-ui, sans-serif;
      letter-spacing: 0.08em;
      text-transform: uppercase;
    }
    .body {
      padding: 1rem 1.1rem 1.2rem;
    }
    .product-link {
      display: block;
      color: inherit;
      text-decoration: none;
    }
    .product-link:focus-visible,
    .back-link:focus-visible,
    select:focus-visible,
    button:focus-visible {
      outline: 3px solid #a16207;
      outline-offset: 3px;
    }
    .product-link h2:hover {
      text-decoration: underline;
      text-underline-offset: 0.2em;
    }
    .detail-view {
      max-width: 62rem;
      margin: 0 auto 2rem;
    }
    .back-link {
      display: inline-block;
      margin: 0 0 1.25rem;
      color: #57534e;
      font: 600 0.9rem/1.5 system-ui, sans-serif;
      text-decoration-thickness: 1px;
      text-underline-offset: 0.2em;
    }
    .detail-layout {
      display: grid;
      grid-template-columns: minmax(0, 1.1fr) minmax(18rem, 0.9fr);
      overflow: hidden;
      border: 1px solid #e7e2d8;
      border-radius: 0.9rem;
      background: #fffefa;
    }
    .detail-layout .placeholder,
    .detail-layout img {
      height: 100%;
      min-height: 22rem;
      aspect-ratio: auto;
    }
    .detail-panel {
      padding: clamp(1.25rem, 4vw, 2.5rem);
    }
    .detail-panel h2 {
      font-size: clamp(1.5rem, 3vw, 2rem);
      line-height: 1.15;
    }
    .detail-panel .description {
      margin-top: 1rem;
      font-size: 1rem;
    }
    .detail-price {
      margin: 1.5rem 0;
      font: 600 1.35rem/1.4 system-ui, sans-serif;
      font-variant-numeric: tabular-nums;
    }
    .variant-fields {
      display: grid;
      gap: 1rem;
      margin: 1.25rem 0;
    }
    .variant-fields label {
      display: grid;
      gap: 0.4rem;
      font: 600 0.9rem/1.4 system-ui, sans-serif;
    }
    .variant-fields select {
      min-height: 2.75rem;
      padding: 0.5rem 0.75rem;
      border: 1px solid #d6cdbd;
      border-radius: 0.35rem;
      background: #fff;
      color: #292524;
      font: 400 1rem/1.4 system-ui, sans-serif;
    }
    .detail-stock {
      margin: 0.75rem 0 0;
      color: #57534e;
      font: 0.85rem/1.5 system-ui, sans-serif;
    }
    @media (max-width: 42rem) {
      .detail-layout {
        grid-template-columns: 1fr;
      }
      .detail-layout .placeholder,
      .detail-layout img {
        min-height: 16rem;
        max-height: 24rem;
      }
    }
    h2 {
      margin: 0;
      font-size: 1.1rem;
      font-weight: 600;
    }
    .description {
      margin: 0.55rem 0 0;
      color: #57534e;
      font: 0.9rem/1.55 system-ui, sans-serif;
    }
    .product-badge {
      display: inline-block;
      margin-top: 0.8rem;
      padding: 0.3rem 0.55rem;
      border-radius: 99rem;
      background: #eee8dc;
      color: #57534e;
      font: 600 0.72rem/1.3 system-ui, sans-serif;
    }
    .price {
      margin: 0.9rem 0 0;
      color: #292524;
      font: 600 0.95rem/1.4 system-ui, sans-serif;
      font-variant-numeric: tabular-nums;
    }
    .add-button,
    .cart-toggle {
      min-height: 2.75rem;
      border: 0;
      border-radius: 0.4rem;
      background: #292524;
      color: #fffefa;
      cursor: pointer;
      font: 600 0.9rem/1.3 system-ui, sans-serif;
    }
    .add-button {
      width: 100%;
      margin-top: 1rem;
      padding: 0.7rem 1rem;
    }
    .add-button:hover,
    .cart-toggle:hover {
      background: #57534e;
    }
    .add-button:disabled {
      cursor: wait;
      opacity: 0.65;
    }
    .cart {
      margin: 1.5rem 0;
      padding: 1.25rem;
      border: 1px solid #d6cdbd;
      border-radius: 0.75rem;
      background: #fffefa;
    }
    .cart-heading {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 1rem;
    }
    .cart-heading h2 {
      margin: 0;
      font-size: 1.2rem;
    }
    .cart-toggle {
      padding: 0.65rem 1rem;
    }
    .cart-content[hidden] {
      display: none;
    }
    .cart-status {
      margin: 0.75rem 0 0;
      color: #57534e;
      font: 0.9rem/1.5 system-ui, sans-serif;
    }
    .cart-status[data-kind="error"] {
      color: #9f1239;
    }
    .cart-items {
      margin: 1rem 0 0;
      padding: 0;
      list-style: none;
      font: 0.9rem/1.5 system-ui, sans-serif;
    }
    .cart-item {
      display: flex;
      justify-content: space-between;
      gap: 1rem;
      padding: 0.7rem 0;
      border-bottom: 1px solid #e7e2d8;
    }
    .cart-item p {
      margin: 0;
    }
    .cart-line-total,
    .cart-total {
      font-variant-numeric: tabular-nums;
      white-space: nowrap;
    }
    .cart-line-total {
      color: #57534e;
    }
    .cart-total-row {
      display: flex;
      justify-content: space-between;
      gap: 1rem;
      margin-top: 1rem;
      font: 600 1rem/1.5 system-ui, sans-serif;
    }
    .checkout-form {
      display: grid;
      gap: 0.85rem;
      margin-top: 1.5rem;
      padding-top: 1.25rem;
      border-top: 1px solid #e7e2d8;
      font: 0.9rem/1.45 system-ui, sans-serif;
    }
    .checkout-form[hidden] {
      display: none;
    }
    .checkout-form h3 {
      margin: 0;
      font-size: 1.05rem;
    }
    .checkout-form label {
      display: grid;
      gap: 0.35rem;
      font-weight: 600;
    }
    .checkout-form input {
      box-sizing: border-box;
      width: 100%;
      min-height: 2.7rem;
      padding: 0.55rem 0.7rem;
      border: 1px solid #d6cdbd;
      border-radius: 0.35rem;
      background: #fff;
      color: #292524;
      font: 400 1rem/1.4 system-ui, sans-serif;
    }
    .checkout-form input:focus-visible {
      outline: 3px solid #a16207;
      outline-offset: 2px;
    }
    .checkout-grid {
      display: grid;
      grid-template-columns: minmax(0, 1fr) minmax(7rem, 0.45fr);
      gap: 0.85rem;
    }
    .checkout-note {
      margin: 0;
      color: #57534e;
      font-size: 0.82rem;
    }
    .checkout-submit {
      min-height: 2.8rem;
      padding: 0.7rem 1rem;
      border: 0;
      border-radius: 0.4rem;
      background: #292524;
      color: #fffefa;
      cursor: pointer;
      font: 600 0.9rem/1.3 system-ui, sans-serif;
    }
    .checkout-submit:hover {
      background: #57534e;
    }
    .checkout-submit:disabled {
      cursor: wait;
      opacity: 0.65;
    }
    @media (max-width: 32rem) {
      .checkout-grid {
        grid-template-columns: 1fr;
      }
    }
    @media (prefers-reduced-motion: reduce) {
      article {
        transition: none;
      }
    }
  </style>
  <h2 class="collection-title">La collection</h2>
  <nav class="category-toolbar" aria-label="Filtres de catalogue" hidden></nav>
  <p class="status" part="status" role="status" aria-live="polite">Chargement du catalogue…</p>
  <section class="cart" aria-labelledby="cart-title">
    <div class="cart-heading">
      <h2 id="cart-title">Votre panier</h2>
      <button class="cart-toggle" type="button" aria-expanded="false">Panier (0)</button>
    </div>
    <div class="cart-content" hidden>
      <p class="cart-status" role="status" aria-live="polite">Votre panier est vide.</p>
      <ul class="cart-items"></ul>
      <p class="cart-total-row" hidden><span>Total</span><span class="cart-total"></span></p>
      <form class="checkout-form" hidden>
        <h3>Coordonnées de livraison</h3>
        <p class="checkout-note">Les frais sont calculés selon la destination et le transporteur sélectionné. Le paiement et les taxes ne sont pas encore calculés ; le stock est déduit à la confirmation.</p>
        <label>Nom complet<input name="name" autocomplete="name" maxlength="120" required /></label>
        <label>Adresse e-mail<input name="email" type="email" autocomplete="email" maxlength="320" required /></label>
        <label>Adresse<input name="line1" autocomplete="address-line1" maxlength="200" required /></label>
        <label>Complément d’adresse<input name="line2" autocomplete="address-line2" maxlength="200" /></label>
        <div class="checkout-grid">
          <label>Ville<input name="city" autocomplete="address-level2" maxlength="100" required /></label>
          <label>Code postal<input name="postal_code" autocomplete="postal-code" maxlength="32" required /></label>
        </div>
        <div class="checkout-grid">
          <label>Région<input name="region" autocomplete="address-level1" maxlength="100" /></label>
          <label>Pays (code ISO)<input name="country" autocomplete="country" maxlength="2" pattern="[A-Za-z]{2}" required /></label>
        </div>
        <label>Mode de livraison
          <select name="shipping_carrier_code" required disabled>
            <option value="">Saisissez un pays pour voir les modes disponibles</option>
          </select>
        </label>
        <p class="checkout-note shipping-options-status" role="status" aria-live="polite"></p>
        <p class="checkout-note checkout-estimate" hidden></p>
        <button class="checkout-submit" type="submit">Confirmer la commande</button>
      </form>
    </div>
  </section>
  <ul class="product-list" aria-label="Produits" hidden></ul>
  <section class="detail-view" aria-live="polite" hidden></section>
`;

const cartTokenKey = "euromni.cart.token";

function translationFor(product, locale) {
  const translations = Array.isArray(product.translations) ? product.translations : [];
  const language = locale.split("-")[0].toLowerCase();
  return translations.find((item) => item.locale === locale)
    || translations.find((item) => item.locale && item.locale.split("-")[0].toLowerCase() === language)
    || translations[0]
    || { name: product.slug || "Produit", description: "" };
}

function priceFor(product, locale) {
  const amounts = (Array.isArray(product.variants) ? product.variants : [])
    .flatMap((variant) => Array.isArray(variant.prices) ? variant.prices : [])
    .filter((price) => price.amount != null && price.amount !== "")
    .sort((left, right) => Number(left.amount) - Number(right.amount));
  const price = amounts[0];
  if (!price) {
    return "";
  }
  const amount = Number(price.amount);
  if (!Number.isFinite(amount)) {
    return "";
  }
  const formatted = formatPrice(price.amount, price.currency, locale);
  const sameCurrencyAmounts = amounts.filter((entry) => entry.currency === price.currency);
  const highest = sameCurrencyAmounts[sameCurrencyAmounts.length - 1];
  if (highest && Number(highest.amount) > amount) {
    return `À partir de ${formatted}`;
  }
  return formatted;
}

function formatPrice(amount, currency, locale) {
  const value = Number(amount);
  if (!Number.isFinite(value)) {
    return `${amount} ${currency || ""}`.trim();
  }
  if (!currency) {
    return String(amount);
  }
  try {
    return new Intl.NumberFormat(locale, { style: "currency", currency }).format(value);
  } catch {
    return `${amount} ${currency}`;
  }
}

function addDecimalAmounts(left, right) {
  const parse = (value) => {
    const match = /^([0-9]+)(?:\.([0-9]{1,4}))?$/.exec(String(value));
    if (!match) {
      throw new Error("Invalid decimal amount");
    }
    return BigInt(match[1]) * 10000n + BigInt((match[2] || "").padEnd(4, "0"));
  };
  const units = parse(left) + parse(right);
  return `${units / 10000n}.${String(units % 10000n).padStart(4, "0")}`;
}

class EuromniCatalog extends HTMLElement {
  static get observedAttributes() {
    return ["locale", "src", "cart-src", "checkout-src", "shipping-src"];
  }

  constructor() {
    super();
    this.attachShadow({ mode: "open" });
    this.shadowRoot.append(template.content.cloneNode(true));
    this.requestController = null;
    this.productsByVariant = new Map();
    this.cartToken = null;
    this.cartBusy = false;
    this.currentCart = null;
    this.shippingOptions = [];
    this.shippingController = null;
    this.detailController = null;
    this.categories = [];
    this.categoryFilter = "";
    this.onPopState = () => this.showRoute();
  }

  connectedCallback() {
    this.shadowRoot.querySelector(".cart-toggle").addEventListener("click", () => this.toggleCart());
    this.shadowRoot.querySelector(".checkout-form").addEventListener("submit", (event) => {
      event.preventDefault();
      this.submitCheckout(event.currentTarget);
    });
    this.shadowRoot.querySelector('[name="country"]').addEventListener("change", (event) => {
      this.loadShippingOptions(event.currentTarget.value);
    });
    this.shadowRoot.querySelector('[name="shipping_carrier_code"]').addEventListener("change", () => {
      this.updateCheckoutEstimate();
    });
    window.addEventListener("popstate", this.onPopState);
    const params = new URLSearchParams(window.location.search);
    if (params.get("category")) {
      this.categoryFilter = params.get("category");
    }
    this.loadCategories();
    this.restoreCart();
    this.load();
  }

  disconnectedCallback() {
    this.requestController?.abort();
    this.detailController?.abort();
    this.shippingController?.abort();
    window.removeEventListener("popstate", this.onPopState);
  }

  attributeChangedCallback(name) {
    if (this.isConnected) {
      if (name === "shipping-src") {
        this.loadShippingOptions(this.shadowRoot.querySelector('[name="country"]').value);
      } else {
        this.load();
      }
    }
  }

  get locale() {
    return this.getAttribute("locale") || "fr-FR";
  }

  get src() {
    return this.getAttribute("src") || "/v1/products";
  }

  get cartSrc() {
    return this.getAttribute("cart-src") || "/v1/cart";
  }

  get checkoutSrc() {
    return this.getAttribute("checkout-src") || "/v1/checkout";
  }

  get shippingSrc() {
    return this.getAttribute("shipping-src") || "/v1/shipping-options";
  }

  async loadShippingOptions(countryValue) {
    const country = String(countryValue || "").trim().toUpperCase();
    const select = this.shadowRoot.querySelector('[name="shipping_carrier_code"]');
    const status = this.shadowRoot.querySelector(".shipping-options-status");
    this.shippingController?.abort();
    this.shippingOptions = [];
    select.replaceChildren();
    select.disabled = true;
    const placeholder = document.createElement("option");
    placeholder.value = "";
    placeholder.textContent = "Chargement des modes de livraison…";
    select.append(placeholder);
    this.updateCheckoutEstimate();
    if (!/^[A-Z]{2}$/.test(country)) {
      placeholder.textContent = "Saisissez un pays valide pour afficher les modes disponibles";
      status.textContent = "";
      return;
    }

    const controller = new AbortController();
    this.shippingController = controller;
    status.textContent = "";
    try {
      const endpoint = new URL(this.shippingSrc, window.location.href);
      endpoint.searchParams.set("country", country);
      const response = await fetch(endpoint, {
        headers: { Accept: "application/json" },
        signal: controller.signal,
      });
      const body = await response.json();
      if (!response.ok) {
        throw new Error(body?.error || "Les modes de livraison sont indisponibles.");
      }
      if (controller.signal.aborted) return;
      this.shippingOptions = Array.isArray(body.data) ? body.data : [];
      select.replaceChildren();
      const prompt = document.createElement("option");
      prompt.value = "";
      prompt.textContent = this.shippingOptions.length
        ? "Choisissez un mode de livraison"
        : "Aucun mode de livraison configuré pour cette destination";
      select.append(prompt);
      for (const option of this.shippingOptions) {
        const item = document.createElement("option");
        item.value = option.carrier_code;
        item.textContent =
          `${option.carrier_name} — ${formatPrice(option.amount, option.currency, this.locale)}`;
        select.append(item);
      }
      select.disabled = this.shippingOptions.length === 0;
      status.textContent = this.shippingOptions.length
        ? ""
        : "La boutique ne livre pas encore cette destination.";
      this.updateCheckoutEstimate();
    } catch (error) {
      if (error.name === "AbortError") return;
      select.replaceChildren();
      const failed = document.createElement("option");
      failed.value = "";
      failed.textContent = "Modes de livraison indisponibles";
      select.append(failed);
      status.textContent = error.message || "Impossible de charger les modes de livraison.";
    } finally {
      if (this.shippingController === controller) this.shippingController = null;
    }
  }

  updateCheckoutEstimate() {
    const estimate = this.shadowRoot.querySelector(".checkout-estimate");
    const carrierCode = this.shadowRoot.querySelector('[name="shipping_carrier_code"]').value;
    const option = this.shippingOptions.find((entry) => entry.carrier_code === carrierCode);
    const cartAmount = this.currentCart?.total?.amount;
    const currency = this.currentCart?.total?.currency || option?.currency;
    if (!option || !currency || !Number.isFinite(Number(cartAmount))) {
      estimate.hidden = true;
      estimate.textContent = "";
      return;
    }
    const grandTotal = addDecimalAmounts(cartAmount, option.amount);
    estimate.hidden = false;
    estimate.textContent =
      `Sous-total ${formatPrice(cartAmount, currency, this.locale)} + livraison ` +
      `${formatPrice(option.amount, option.currency, this.locale)} = ` +
      `${formatPrice(grandTotal, currency, this.locale)} (hors taxes).`;
  }

  async restoreCart() {
    try {
      this.cartToken = sessionStorage.getItem(cartTokenKey);
    } catch {
      this.setCartStatus("Le stockage de session est indisponible ; le panier ne peut pas être restauré.", true);
      return;
    }
    if (this.cartToken) {
      await this.refreshCart();
    }
  }

  async load() {
    const status = this.shadowRoot.querySelector(".status");
    const list = this.shadowRoot.querySelector(".product-list");
    this.requestController?.abort();
    const controller = new AbortController();
    this.requestController = controller;
    status.hidden = false;
    status.dataset.kind = "loading";
    status.textContent = "Chargement du catalogue…";
    list.hidden = true;
    list.replaceChildren();

    try {
      const endpoint = new URL(this.src, window.location.href);
      if (this.categoryFilter) {
        endpoint.searchParams.set("category", this.categoryFilter);
      } else {
        endpoint.searchParams.delete("category");
      }
      const response = await fetch(endpoint, {
        headers: { Accept: "application/json" },
        signal: controller.signal,
      });
      if (!response.ok) {
        throw new Error("catalog request failed");
      }
      const body = await response.json();
      const products = Array.isArray(body.data) ? body.data : [];
      if (controller.signal.aborted) {
        return;
      }
      if (products.length === 0) {
        this.productsByVariant.clear();
        status.textContent = "Aucun produit actif.";
        return;
      }
      this.productsByVariant.clear();
      for (const product of products) {
        for (const variant of Array.isArray(product.variants) ? product.variants : []) {
          this.productsByVariant.set(variant.id, {
            name: translationFor(product, this.locale).name,
          });
        }
        list.append(this.renderProduct(product));
      }
      if (this.currentCart) {
        this.renderCart(this.currentCart);
      }
      status.hidden = true;
      list.hidden = false;
      await this.showRoute();
    } catch (error) {
      if (error.name === "AbortError") {
        return;
      }
      status.dataset.kind = "error";
      status.textContent = "Catalogue indisponible.";
    } finally {
      if (this.requestController === controller) {
        this.requestController = null;
      }
    }
  }

  async loadCategories() {
    try {
      const response = await fetch("/v1/categories", { headers: { Accept: "application/json" } });
      if (!response.ok) {
        throw new Error("categories request failed");
      }
      const body = await response.json();
      this.categories = Array.isArray(body.data) ? body.data : [];
      this.renderCategoryFilters();
    } catch {
      this.categories = [];
      this.renderCategoryFilters();
    }
  }

  renderCategoryFilters() {
    const nav = this.shadowRoot.querySelector(".category-toolbar");
    nav.replaceChildren();
    if (!this.categories.length) {
      nav.hidden = true;
      return;
    }

    const allButton = document.createElement("button");
    allButton.type = "button";
    allButton.className = "category-button";
    allButton.textContent = "Tout voir";
    allButton.setAttribute("aria-pressed", String(!this.categoryFilter));
    allButton.addEventListener("click", () => {
      this.categoryFilter = "";
      this.renderCategoryFilters();
      this.load();
    });
    nav.append(allButton);

    for (const category of this.categories) {
      const translation = Array.isArray(category.translations) ? category.translations.find((entry) => entry.locale === this.locale) || category.translations[0] : null;
      const button = document.createElement("button");
      button.type = "button";
      button.className = "category-button";
      button.textContent = translation ? translation.name : category.slug;
      button.setAttribute("aria-pressed", String(this.categoryFilter === category.id));
      button.addEventListener("click", () => {
        this.categoryFilter = this.categoryFilter === category.id ? "" : category.id;
        this.renderCategoryFilters();
        this.load();
      });
      nav.append(button);
    }
    nav.hidden = false;
  }

  renderProduct(product) {
    const item = document.createElement("li");
    const article = document.createElement("article");
    const translation = translationFor(product, this.locale);
    const media = Array.isArray(product.media) ? product.media[0] : null;
    if (media && media.uri) {
      const image = document.createElement("img");
      image.src = media.uri;
      image.alt = media.alt || translation.name;
      image.loading = "lazy";
      image.decoding = "async";
      image.addEventListener("error", () => image.replaceWith(this.createPlaceholder(translation.name)), { once: true });
      article.append(image);
    } else {
      article.append(this.createPlaceholder(translation.name));
    }
    const body = document.createElement("div");
    body.className = "body";
    const link = document.createElement("a");
    link.className = "product-link";
    link.href = this.productHref(product.id);
    const title = document.createElement("h2");
    title.textContent = translation.name;
    link.append(title);
    link.addEventListener("click", (event) => {
      if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
        return;
      }
      event.preventDefault();
      this.navigateToProduct(product.id);
    });
    body.append(link);
    const variants = Array.isArray(product.variants) ? product.variants : [];
    const badge = document.createElement("span");
    badge.className = "product-badge";
    badge.textContent = variants.length > 1 ? "Tailles au choix" : "Format unique";
    body.append(badge);
    if (translation.description) {
      const description = document.createElement("p");
      description.className = "description";
      description.textContent = translation.description;
      body.append(description);
    }
    const amount = priceFor(product, this.locale);
    if (amount) {
      const price = document.createElement("p");
      price.className = "price";
      price.setAttribute("aria-label", `Prix : ${amount}`);
      price.textContent = amount;
      body.append(price);
    }
    const detailLink = document.createElement("a");
    detailLink.className = "back-link";
    detailLink.href = this.productHref(product.id);
    detailLink.textContent = "Découvrir le produit";
    detailLink.addEventListener("click", (event) => {
      if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
        return;
      }
      event.preventDefault();
      this.navigateToProduct(product.id);
    });
    body.append(detailLink);
    article.append(body);
    item.append(article);
    return item;
  }

  toggleCart() {
    const button = this.shadowRoot.querySelector(".cart-toggle");
    const content = this.shadowRoot.querySelector(".cart-content");
    const expanded = button.getAttribute("aria-expanded") === "true";
    button.setAttribute("aria-expanded", String(!expanded));
    content.hidden = expanded;
  }

  setCartStatus(message, isError = false) {
    const status = this.shadowRoot.querySelector(".cart-status");
    status.textContent = message;
    status.dataset.kind = isError ? "error" : "info";
  }

  async requestCart(path, options = {}) {
    const headers = new Headers(options.headers || {});
    headers.set("Accept", "application/json");
    if (this.cartToken) {
      headers.set("Authorization", `Bearer ${this.cartToken}`);
    }
    const response = await fetch(path, { ...options, headers });
    let body;
    try {
      body = await response.json();
    } catch {
      throw new Error("Réponse invalide du service panier.");
    }
    if (!response.ok) {
      if (response.status === 409) {
        throw new Error(body && body.error === "cart is empty"
          ? "Votre panier est vide."
          : body && body.error === "insufficient inventory"
            ? "Le stock ne permet plus cette commande."
            : body && body.error === "shipping method unavailable for this destination"
              ? "Ce mode de livraison n’est plus disponible pour cette destination. Choisissez-en un autre."
            : "Cette commande ne peut pas être finalisée.");
      }
      if (response.status === 401 && this.cartToken) {
        this.clearCartToken();
        throw new Error("Votre panier a expiré. Ajoutez à nouveau un article pour en créer un nouveau.");
      }
      throw new Error((body && body.error) || "Le service panier est indisponible.");
    }
    return body.data;
  }

  async ensureCart() {
    if (this.cartToken) {
      return;
    }
    const cart = await this.requestCart(this.cartSrc, { method: "POST" });
    if (!cart || typeof cart.token !== "string" || !cart.token) {
      throw new Error("Le service panier a renvoyé un jeton invalide.");
    }
    this.cartToken = cart.token;
    try {
      sessionStorage.setItem(cartTokenKey, this.cartToken);
    } catch {
      this.cartToken = null;
      throw new Error("Impossible de conserver le panier dans cette session.");
    }
  }

  clearCartToken() {
    this.cartToken = null;
    try {
      sessionStorage.removeItem(cartTokenKey);
    } catch {
      this.setCartStatus("Le panier a expiré, mais son jeton n’a pas pu être effacé de la session.", true);
    }
    this.renderCart({ items: [], total: { amount: "0", currency: "" } });
  }

  async addToCart(variantID, button) {
    if (this.cartBusy) {
      return;
    }
    this.cartBusy = true;
    const buttons = [...this.shadowRoot.querySelectorAll(".add-button")];
    const previousDisabled = new Map(buttons.map((addButton) => [addButton, addButton.disabled]));
    for (const addButton of buttons) {
      addButton.disabled = true;
    }
    button.textContent = "Ajout en cours…";
    try {
      await this.ensureCart();
      const cart = await this.requestCart(`${this.cartSrc}/add`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ variant_id: variantID, quantity: 1 }),
      });
      this.renderCart(cart);
      this.setCartStatus("Article ajouté à votre panier.");
      const content = this.shadowRoot.querySelector(".cart-content");
      const toggle = this.shadowRoot.querySelector(".cart-toggle");
      content.hidden = false;
      toggle.setAttribute("aria-expanded", "true");
    } catch (error) {
      this.setCartStatus(error.message, true);
      this.shadowRoot.querySelector(".cart-content").hidden = false;
      this.shadowRoot.querySelector(".cart-toggle").setAttribute("aria-expanded", "true");
    } finally {
      this.cartBusy = false;
      button.textContent = "Ajouter au panier";
      for (const addButton of buttons) {
        addButton.disabled = previousDisabled.get(addButton);
      }
    }
  }

  async refreshCart() {
    try {
      const cart = await this.requestCart(this.cartSrc);
      this.renderCart(cart);
      if (cart.items.length > 0) {
        this.setCartStatus("");
      }
    } catch (error) {
      this.setCartStatus(error.message, true);
    }
  }

  async submitCheckout(form) {
    if (this.cartBusy) {
      return;
    }
    this.cartBusy = true;
    const button = form.querySelector(".checkout-submit");
    button.disabled = true;
    button.textContent = "Enregistrement…";
    const values = Object.fromEntries(new FormData(form).entries());
    const details = {
      email: values.email,
      name: values.name,
      shipping_carrier_code: values.shipping_carrier_code,
      shipping_address: {
        line1: values.line1,
        line2: values.line2,
        city: values.city,
        postal_code: values.postal_code,
        region: values.region,
        country: values.country,
      },
    };
    try {
      const order = await this.requestCart(this.checkoutSrc, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(details),
      });
      this.renderCart({ items: [], total: { amount: "0.0000", currency: order.currency } });
      form.reset();
      this.shippingOptions = [];
      this.shadowRoot.querySelector('[name="shipping_carrier_code"]').replaceChildren();
      this.shadowRoot.querySelector('[name="shipping_carrier_code"]').disabled = true;
      this.setCartStatus(
        `Commande ${order.number} enregistrée en attente de paiement. Total : ` +
        `${formatPrice(order.grand_total, order.currency, this.locale)} (hors taxes).`,
        false,
      );
    } catch (error) {
      this.setCartStatus(error.message, true);
    } finally {
      this.cartBusy = false;
      button.disabled = false;
      button.textContent = "Confirmer la commande";
    }
  }

  renderCart(cart) {
    this.currentCart = cart;
    const items = this.shadowRoot.querySelector(".cart-items");
    const total = this.shadowRoot.querySelector(".cart-total");
    const totalRow = this.shadowRoot.querySelector(".cart-total-row");
    const checkoutForm = this.shadowRoot.querySelector(".checkout-form");
    const toggle = this.shadowRoot.querySelector(".cart-toggle");
    const cartItems = Array.isArray(cart.items) ? cart.items : [];
    const itemCount = cartItems.reduce((sum, item) => sum + item.quantity, 0);
    toggle.textContent = `Panier (${itemCount})`;
    items.replaceChildren();
    for (const item of cartItems) {
      const row = document.createElement("li");
      row.className = "cart-item";
      const info = document.createElement("p");
      const product = this.productsByVariant.get(item.variant_id);
      info.textContent = `${product ? product.name : item.sku} × ${item.quantity}`;
      const lineTotal = document.createElement("span");
      lineTotal.className = "cart-line-total";
      lineTotal.textContent = `${formatPrice(item.unit_amount, cart.total.currency, this.locale)} / unité`;
      row.append(info, lineTotal);
      items.append(row);
    }
    const hasItems = cartItems.length > 0;
    totalRow.hidden = !hasItems;
    checkoutForm.hidden = !hasItems;
    total.textContent = formatPrice(cart.total.amount, cart.total.currency, this.locale);
    this.updateCheckoutEstimate();
    if (!hasItems) {
      this.setCartStatus("Votre panier est vide.");
    }
  }

  productHref(productID) {
    const url = new URL(window.location.href);
    if (this.categoryFilter) {
      url.searchParams.set("category", this.categoryFilter);
    } else {
      url.searchParams.delete("category");
    }
    url.searchParams.set("product", productID);
    return `${url.pathname}${url.search}${url.hash}`;
  }

  navigateToProduct(productID) {
    window.history.pushState({}, "", this.productHref(productID));
    this.showRoute();
    window.scrollTo({ top: 0, behavior: "smooth" });
  }

  async showRoute() {
    const params = new URLSearchParams(window.location.search);
    const categoryID = params.get("category");
    const productID = params.get("product");
    if (categoryID) {
      this.categoryFilter = categoryID;
    } else {
      this.categoryFilter = "";
    }
    const list = this.shadowRoot.querySelector(".product-list");
    const detail = this.shadowRoot.querySelector(".detail-view");
    if (!productID) {
      this.detailController?.abort();
      this.detailController = null;
      detail.hidden = true;
      detail.replaceChildren();
      list.hidden = false;
      this.shadowRoot.querySelector(".collection-title").hidden = false;
      document.title = "Atelier Demo — EUROMNI";
      return;
    }
    this.shadowRoot.querySelector(".collection-title").hidden = true;
    list.hidden = true;
    detail.hidden = false;
    await this.showProduct(productID);
  }

  async showProduct(productID) {
    this.detailController?.abort();
    const controller = new AbortController();
    this.detailController = controller;
    const detail = this.shadowRoot.querySelector(".detail-view");
    detail.hidden = false;
    detail.replaceChildren();
    const loading = document.createElement("p");
    loading.className = "status";
    loading.textContent = "Chargement de la fiche produit…";
    detail.append(loading);
    try {
      const endpoint = new URL(this.src, window.location.href);
      endpoint.pathname = `${endpoint.pathname.replace(/\/$/, "")}/${encodeURIComponent(productID)}`;
      endpoint.search = "";
      const response = await fetch(endpoint, {
        headers: { Accept: "application/json" },
        signal: controller.signal,
      });
      if (response.status === 404) {
        throw new Error("Ce produit n’est pas disponible.");
      }
      if (!response.ok) {
        throw new Error("Impossible de charger cette fiche produit.");
      }
      const body = await response.json();
      if (controller.signal.aborted) {
        return;
      }
      const product = body.data;
      detail.replaceChildren(this.renderProductDetail(product));
      document.title = `${translationFor(product, this.locale).name} — Atelier Demo`;
    } catch (error) {
      if (error.name === "AbortError") {
        return;
      }
      detail.replaceChildren();
      const back = document.createElement("a");
      back.className = "back-link";
      back.href = window.location.pathname;
      back.textContent = "← Retour à la collection";
      back.addEventListener("click", (event) => {
        event.preventDefault();
        this.navigateToCatalog();
      });
      const message = document.createElement("p");
      message.className = "status";
      message.dataset.kind = "error";
      message.textContent = error.message;
      detail.append(back, message);
    } finally {
      if (this.detailController === controller) {
        this.detailController = null;
      }
    }
  }

  navigateToCatalog() {
    const url = new URL(window.location.href);
    url.searchParams.delete("product");
    if (this.categoryFilter) {
      url.searchParams.set("category", this.categoryFilter);
    }
    window.history.pushState({}, "", `${url.pathname}${url.search}${url.hash}`);
    this.showRoute();
  }

  renderProductDetail(product) {
    const wrapper = document.createElement("div");
    const back = document.createElement("a");
    back.className = "back-link";
    back.href = window.location.pathname;
    back.textContent = "← Retour à la collection";
    back.addEventListener("click", (event) => {
      event.preventDefault();
      this.navigateToCatalog();
    });

    const layout = document.createElement("div");
    layout.className = "detail-layout";
    const translation = translationFor(product, this.locale);
    const media = Array.isArray(product.media) ? product.media[0] : null;
    if (media && media.uri) {
      const image = document.createElement("img");
      image.src = media.uri;
      image.alt = media.alt || translation.name;
      image.addEventListener("error", () => image.replaceWith(this.createPlaceholder(translation.name)), { once: true });
      layout.append(image);
    } else {
      layout.append(this.createPlaceholder(translation.name));
    }

    const panel = document.createElement("div");
    panel.className = "detail-panel";
    const title = document.createElement("h2");
    title.textContent = translation.name;
    panel.append(title);
    if (translation.description) {
      const description = document.createElement("p");
      description.className = "description";
      description.textContent = translation.description;
      panel.append(description);
    }
    const variants = Array.isArray(product.variants) ? product.variants : [];
    const fields = document.createElement("div");
    fields.className = "variant-fields";
    const optionNames = [...new Set(variants.flatMap((variant) => Object.keys(variant.options || {})))]
      .filter((name) => new Set(variants.map((variant) => String(variant.options[name]))).size > 1);
    if (optionNames.length === 0) {
      const format = document.createElement("span");
      format.className = "product-badge";
      format.textContent = "Format unique";
      fields.append(format);
    }
    for (const optionName of optionNames) {
      const label = document.createElement("label");
      const labelText = document.createElement("span");
      labelText.textContent = this.optionLabel(optionName);
      const select = document.createElement("select");
      select.dataset.option = optionName;
      const values = [...new Set(variants.map((variant) => variant.options[optionName]).filter((value) => value != null))]
        .sort((left, right) => this.compareOptionValues(optionName, left, right));
      for (const value of values) {
        const option = document.createElement("option");
        option.value = String(value);
        option.textContent = String(value);
        select.append(option);
      }
      label.append(labelText, select);
      fields.append(label);
    }
    panel.append(fields);

    const price = document.createElement("p");
    price.className = "detail-price";
    panel.append(price);
    const stock = document.createElement("p");
    stock.className = "detail-stock";
    panel.append(stock);
    const addButton = document.createElement("button");
    addButton.className = "add-button";
    addButton.type = "button";
    addButton.textContent = "Ajouter au panier";
    panel.append(addButton);

    const updateSelection = () => {
      const selected = Object.fromEntries([...fields.querySelectorAll("select")].map((select) => [select.dataset.option, select.value]));
      const variant = variants.find((candidate) => Object.entries(selected).every(([key, value]) => String(candidate.options[key]) === value));
      const variantPrice = variant && Array.isArray(variant.prices) ? variant.prices[0] : null;
      price.textContent = variantPrice ? formatPrice(variantPrice.amount, variantPrice.currency, this.locale) : "Indisponible";
      const available = !variant || !variant.inventory.track || variant.inventory.quantity > 0;
      stock.textContent = !variant ? "Cette combinaison n’est pas disponible."
        : variant.inventory.track ? (available ? `${variant.inventory.quantity} en stock` : "Rupture de stock")
          : "Disponible";
      addButton.disabled = !variant || !available;
      addButton.dataset.variantId = variant ? variant.id : "";
    };
    fields.addEventListener("change", updateSelection);
    addButton.addEventListener("click", () => {
      if (addButton.dataset.variantId) {
        this.addToCart(addButton.dataset.variantId, addButton);
      }
    });
    updateSelection();

    layout.append(panel);
    wrapper.append(back, layout);
    return wrapper;
  }

  optionLabel(key) {
    const labels = { size: "Taille", taille: "Taille", color: "Couleur", couleur: "Couleur" };
    return labels[key.toLowerCase()] || key;
  }

  compareOptionValues(key, left, right) {
    const sizeOrder = ["XXS", "XS", "S", "M", "L", "XL", "XXL", "XXXL"];
    if (["size", "taille"].includes(key.toLowerCase())) {
      const leftSize = sizeOrder.indexOf(String(left).toUpperCase());
      const rightSize = sizeOrder.indexOf(String(right).toUpperCase());
      if (leftSize !== -1 && rightSize !== -1) {
        return leftSize - rightSize;
      }
      const leftNumber = Number(left);
      const rightNumber = Number(right);
      if (Number.isFinite(leftNumber) && Number.isFinite(rightNumber)) {
        return leftNumber - rightNumber;
      }
    }
    return String(left).localeCompare(String(right), this.locale, { numeric: true, sensitivity: "base" });
  }

  createPlaceholder(label = "Atelier EUROMNI") {
    const placeholder = document.createElement("div");
    placeholder.className = "placeholder";
    placeholder.setAttribute("aria-hidden", "true");
    placeholder.textContent = label;
    return placeholder;
  }
}

customElements.define("euromni-catalog", EuromniCatalog);
