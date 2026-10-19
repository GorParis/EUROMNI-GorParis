(() => {
  const tokenKey = "euromni.staff.token";
  const loginPanel = document.querySelector("#login-panel");
  const dashboard = document.querySelector("#dashboard");
  const status = document.querySelector("#status");
  const logoutButton = document.querySelector("#logout");
  const productForm = document.querySelector("#product-form");
  const categoryForm = document.querySelector("#category-form");
  const staffForm = document.querySelector("#staff-form");
  const productList = document.querySelector("#products");
  const categoryList = document.querySelector("#categories");
  const categoryOptions = document.querySelector("#category-options");
  const staffList = document.querySelector("#staff-members");
  const orderList = document.querySelector("#orders");
  const orderDetail = document.querySelector("#order-detail");
  const shippingPanel = document.querySelector("#shipping-panel");
  const shippingCarriers = document.querySelector("#shipping-carriers");
  const shippingServices = document.querySelector("#shipping-services");
  const shippingZones = document.querySelector("#shipping-zones");
  const shippingRates = document.querySelector("#shipping-rates");
  const saveShippingButton = document.querySelector("#save-shipping");
  const shippingHistoryFilters = document.querySelector("#shipping-history-filters");
  const shippingHistoryList = document.querySelector("#shipping-history");
  const shippingHistoryPagination = document.querySelector("#shipping-history-pagination");
  const shippingHistoryComparison = document.querySelector("#shipping-history-comparison");
  const shippingHistoryLimit = 25;
  let token = null;
  let categories = [];
  let editingCategory = null;
  let currentIdentity = null;
  let shippingConfig = { carriers: [], services: [], zones: [], rates: [] };
  let shippingHistoryFilter = { carrierCode: "", serviceCode: "", zoneCode: "" };
  let shippingHistoryOffset = 0;
  let shippingHistoryRequest = 0;
  let shippingHistoryTargets = null;
  let shippingHistoryTargetsPromise = null;
  let shippingHistoryTargetsPromiseShopID = null;
  let shippingHistoryTargetsShopID = null;
  let shippingHistoryTargetsGeneration = 0;

  function setStatus(message, isError = false) {
    status.textContent = message;
    status.dataset.kind = isError ? "error" : "info";
  }

  async function request(path, options = {}) {
    const headers = new Headers(options.headers || {});
    headers.set("Accept", "application/json");
    if (token) headers.set("Authorization", `Bearer ${token}`);
    if (options.body) headers.set("Content-Type", "application/json");
    const response = await fetch(path, { ...options, headers });
    if (response.status === 204) return null;
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
      if (response.status === 401) clearSession();
      throw new Error(payload.error || `Erreur HTTP ${response.status}`);
    }
    return payload.data;
  }

  function showDashboard(identity) {
    if (currentIdentity?.shop_id !== identity.shop_id) {
      shippingHistoryTargets = null;
      shippingHistoryTargetsPromise = null;
      shippingHistoryTargetsPromiseShopID = null;
      shippingHistoryTargetsShopID = null;
      shippingHistoryTargetsGeneration += 1;
      shippingHistoryRequest += 1;
    }
    currentIdentity = identity;
    loginPanel.hidden = true;
    dashboard.hidden = false;
    logoutButton.hidden = false;
    document.querySelector("#staff-panel").hidden = identity.role !== "owner";
    document.querySelector("#orders-panel").hidden =
      !["owner", "admin", "support"].includes(identity.role);
    shippingPanel.hidden = !["owner", "admin"].includes(identity.role);
    document.querySelector("#identity").textContent =
      `${identity.name || identity.email} · ${identity.email} · rôle ${identity.role}`;
  }

  function clearSession() {
    token = null;
    shippingHistoryTargets = null;
    shippingHistoryTargetsPromise = null;
    shippingHistoryTargetsPromiseShopID = null;
    shippingHistoryTargetsShopID = null;
    shippingHistoryTargetsGeneration += 1;
    shippingHistoryRequest += 1;
    try {
      sessionStorage.removeItem(tokenKey);
    } catch {
      setStatus("Le stockage de session est indisponible.", true);
    }
    dashboard.hidden = true;
    logoutButton.hidden = true;
    loginPanel.hidden = false;
  }

  function translationLabel(translations) {
    const french = translations?.find((translation) => translation.locale === "fr-FR");
    return french?.name || translations?.[0]?.name || "Sans nom";
  }

  function orderStatusLabel(status) {
    return ({
      pending: "En attente de paiement",
      awaiting_payment: "Paiement attendu",
      paid: "Payée",
      fulfilling: "En préparation",
      fulfilled: "Expédiée",
      cancelled: "Annulée",
      refunded: "Remboursée",
    })[status] || status;
  }

  function shipmentStatusLabel(status) {
    return ({
      pending: "En attente",
      ready: "Prête",
      shipped: "Expédiée",
      delivered: "Livrée",
      returned: "Retournée",
    })[status] || status;
  }

  function renderProducts(products) {
    productList.replaceChildren();
    for (const product of products) {
      const card = document.createElement("article");
      card.className = "card";
      const title = document.createElement("h3");
      title.textContent = translationLabel(product.translations);
      const image = product.media?.[0];
      if (image) {
        const preview = document.createElement("img");
        preview.src = image.uri;
        preview.alt = image.alt || "";
        preview.loading = "lazy";
        preview.style.maxWidth = "8rem";
        preview.style.maxHeight = "8rem";
        preview.style.objectFit = "cover";
        card.append(preview);
      }
      const metadata = document.createElement("p");
      metadata.className = "muted";
      metadata.textContent = `${product.slug} · ${product.status} · ${product.variants.length} variante(s)`;
      const actions = document.createElement("div");
      actions.className = "actions";
      const edit = document.createElement("button");
      edit.type = "button";
      edit.className = "secondary";
      edit.textContent = "Modifier";
      edit.addEventListener("click", () => editProduct(product));
      actions.append(edit);
      if (product.status !== "archived") {
        const archive = document.createElement("button");
        archive.type = "button";
        archive.className = "danger";
        archive.textContent = "Archiver";
        archive.addEventListener("click", async () => {
          if (!confirm(`Archiver « ${title.textContent} » ?`)) return;
          try {
            await request(`/v1/admin/products/${encodeURIComponent(product.id)}`, { method: "DELETE" });
            setStatus("Produit archivé.");
            await refresh();
          } catch (error) {
            setStatus(error.message, true);
          }
        });
        actions.append(archive);
      }
      card.append(title, metadata, actions);
      productList.append(card);
    }
  }

  function renderCategories() {
    categoryList.replaceChildren();
    categoryOptions.replaceChildren();
    for (const category of categories) {
      const label = document.createElement("label");
      label.className = "category-option";
      const checkbox = document.createElement("input");
      checkbox.type = "checkbox";
      checkbox.name = "category_ids";
      checkbox.value = category.id;
      const name = document.createElement("span");
      name.textContent = `${translationLabel(category.translations)} (${category.slug})`;
      label.append(checkbox, name);
      categoryOptions.append(label);

      const card = document.createElement("article");
      card.className = "card";
      const title = document.createElement("h3");
      title.textContent = translationLabel(category.translations);
      const slug = document.createElement("p");
      slug.className = "muted";
      slug.textContent = category.slug;
      const actions = document.createElement("div");
      actions.className = "actions";
      const edit = document.createElement("button");
      edit.type = "button";
      edit.className = "secondary";
      edit.textContent = "Modifier";
      edit.addEventListener("click", () => editCategory(category));
      const remove = document.createElement("button");
      remove.type = "button";
      remove.className = "danger";
      remove.textContent = "Supprimer";
      remove.addEventListener("click", async () => {
        if (!confirm(`Supprimer la catégorie « ${title.textContent} » ? Les liens aux produits seront retirés.`)) return;
        try {
          await request(`/v1/admin/categories/${encodeURIComponent(category.id)}`, { method: "DELETE" });
          setStatus("Catégorie supprimée.");
          await refresh();
        } catch (error) {
          setStatus(error.message, true);
        }
      });
      actions.append(edit, remove);
      card.append(title, slug, actions);
      categoryList.append(card);
    }
  }

  function renderStaff(members) {
    staffList.replaceChildren();
    for (const member of members) {
      const card = document.createElement("article");
      card.className = "card";
      const title = document.createElement("h3");
      title.textContent = `${member.name} · ${member.role}`;
      const email = document.createElement("p");
      email.className = "muted";
      email.textContent = member.email;
      const actions = document.createElement("div");
      actions.className = "actions";
      if (member.role !== "owner") {
        const edit = document.createElement("button");
        edit.type = "button";
        edit.className = "secondary";
        edit.textContent = "Modifier";
        edit.addEventListener("click", () => editStaff(member));
        const remove = document.createElement("button");
        remove.type = "button";
        remove.className = "danger";
        remove.textContent = "Retirer";
        remove.addEventListener("click", async () => {
          if (!confirm(`Retirer ${member.email} de cette boutique et révoquer ses sessions ?`)) return;
          try {
            await request(`/v1/admin/staff/${encodeURIComponent(member.id)}`, { method: "DELETE" });
            setStatus("Membre retiré et sessions révoquées.");
            await refresh();
          } catch (error) {
            setStatus(error.message, true);
          }
        });
        actions.append(edit, remove);
      }
      card.append(title, email, actions);
      staffList.append(card);
    }
  }

  function renderOrderDetail(order) {
    orderDetail.replaceChildren();
    orderDetail.hidden = false;
    const heading = document.createElement("h3");
    heading.textContent = `Commande ${order.number} · ${orderStatusLabel(order.status)}`;
    const customer = document.createElement("p");
    customer.textContent = `${order.name} · ${order.email}`;
    const total = document.createElement("p");
    total.textContent = `Total : ${order.grand_total} ${order.currency}`;
    const shipping = document.createElement("p");
    const serviceLabel = order.shipping_service_name ? ` · ${order.shipping_service_name}` : "";
    const versionLabel = order.shipping_rate_version ? ` · version ${order.shipping_rate_version}` : "";
    shipping.textContent = order.shipping_carrier_name
      ? `Livraison : ${order.shipping_carrier_name}${serviceLabel} · ${order.shipping_total} ${order.currency} · ${order.shipping_zone_name || ""}${versionLabel}`
      : `Livraison : ${order.shipping_total} ${order.currency}`;
    if (order.shipment_status) {
      const shipment = document.createElement("p");
      shipment.textContent = `Expédition : ${shipmentStatusLabel(order.shipment_status)}${order.tracking ? ` · Suivi ${order.tracking}` : ""}`;
      orderDetail.append(heading, customer, shipping, shipment);
    } else {
      orderDetail.append(heading, customer, shipping);
    }
    const items = document.createElement("ul");
    for (const item of order.items) {
      const row = document.createElement("li");
      row.textContent = `${item.quantity} × ${item.name} (${item.sku}) · ${item.line_total} ${order.currency}`;
      items.append(row);
    }
    const address = document.createElement("p");
    address.textContent = `${order.shipping_address.line1 || ""} · ${order.shipping_address.postal_code || ""} ${order.shipping_address.city || ""} · ${order.shipping_address.country || ""}`;
    orderDetail.append(total, items, address);
  }

  function renderOrders(orders) {
    orderList.replaceChildren();
    orderDetail.hidden = true;
    for (const order of orders) {
      const card = document.createElement("article");
      card.className = "card";
      const title = document.createElement("h3");
      title.textContent = `${order.number} · ${orderStatusLabel(order.status)}`;
      const summary = document.createElement("p");
      summary.className = "muted";
      summary.textContent = `${order.name} · ${order.email} · ${order.grand_total} ${order.currency}`;
      const actions = document.createElement("div");
      actions.className = "actions";
      const view = document.createElement("button");
      view.type = "button";
      view.className = "secondary";
      view.textContent = "Détail";
      view.addEventListener("click", async () => {
        try {
          renderOrderDetail(await request(`/v1/admin/orders/${encodeURIComponent(order.id)}`));
        } catch (error) {
          setStatus(error.message, true);
        }

      });
      actions.append(view);
      if (order.status === "pending" && ["owner", "admin"].includes(currentIdentity?.role)) {
        const payment = document.createElement("button");
        payment.type = "button";
        payment.textContent = "Confirmer le paiement reçu";
        payment.addEventListener("click", async () => {
          if (!confirm(`Confirmez-vous avoir reçu le paiement de ${order.grand_total} ${order.currency} pour ${order.number} ? Cette action n’effectue aucun débit.`)) return;
          const providerReference = prompt("Référence du virement ou reçu (facultatif) :", "")?.trim();
          if (providerReference === undefined) return;
          try {
            await request(`/v1/admin/orders/${encodeURIComponent(order.id)}/payments/manual`, {
              method: "POST",
              body: JSON.stringify({ provider_reference: providerReference }),
            });
            setStatus("Paiement manuel enregistré.");
            await refresh();
          } catch (error) {
            setStatus(error.message, true);
          }
        });
        actions.append(payment);
      }
      if (order.status === "paid" && ["owner", "admin"].includes(currentIdentity?.role)) {
        const fulfill = document.createElement("button");
        fulfill.type = "button";
        fulfill.textContent = "Marquer comme expédiée";
        fulfill.addEventListener("click", async () => {
          if (!confirm(`Confirmez-vous la remise de la commande ${order.number} au transporteur ?`)) return;
          const tracking = prompt("Numéro de suivi (facultatif) :", "")?.trim();
          if (tracking === undefined) return;
          try {
            await request(`/v1/admin/orders/${encodeURIComponent(order.id)}/fulfill`, {
              method: "POST",
              body: JSON.stringify({ tracking }),
            });
            setStatus("Commande marquée comme expédiée.");
            await refresh();
          } catch (error) {
            setStatus(error.message, true);
          }
        });
        actions.append(fulfill);
      }
      if (order.status === "pending" && ["owner", "admin"].includes(currentIdentity?.role)) {
        const cancel = document.createElement("button");
        cancel.type = "button";
        cancel.className = "danger";
        cancel.textContent = "Annuler";
        cancel.addEventListener("click", async () => {
          if (!confirm(`Annuler la commande ${order.number} et restituer le stock ?`)) return;
          try {
            await request(`/v1/admin/orders/${encodeURIComponent(order.id)}/status`, {
              method: "PATCH",
              body: JSON.stringify({ status: "cancelled" }),
            });
            setStatus("Commande annulée et stock restitué.");
            await refresh();
          } catch (error) {
            setStatus(error.message, true);
          }
        });
        actions.append(cancel);
      }
      card.append(title, summary, actions);
      orderList.append(card);
    }
  }

  function shippingField(labelText, value, onInput, options = {}) {
    const label = document.createElement("label");
    label.textContent = labelText;
    const input = document.createElement("input");
    input.type = options.type || "text";
    input.value = value ?? "";
    if (options.pattern) input.pattern = options.pattern;
    if (options.required) input.required = true;
    if (options.min !== undefined) input.min = options.min;
    if (options.step !== undefined) input.step = options.step;
    input.addEventListener("input", () => onInput(input.value));
    label.append(input);
    return label;
  }

  function shippingToggle(labelText, checked, onChange) {
    const label = document.createElement("label");
    label.className = "category-option";
    const input = document.createElement("input");
    input.type = "checkbox";
    input.checked = checked;
    input.addEventListener("change", () => onChange(input.checked));
    label.append(input, document.createTextNode(labelText));
    return label;
  }

  function renderShipping(config) {
    shippingConfig = config || { carriers: [], services: [], zones: [], rates: [] };
    for (const key of ["carriers", "services", "zones", "rates"]) {
      if (!Array.isArray(shippingConfig[key])) shippingConfig[key] = [];
    }
    shippingCarriers.replaceChildren();
    shippingServices.replaceChildren();
    shippingZones.replaceChildren();
    shippingRates.replaceChildren();

    shippingConfig.carriers.forEach((carrier, index) => {
      const card = document.createElement("article");
      card.className = "card";
      card.append(
        shippingField("Code", carrier.code, (value) => {
          const previousCode = carrier.code;
          carrier.code = value;
          shippingConfig.services.forEach((service) => {
            if (service.carrier_code === previousCode) service.carrier_code = value;
          });
          shippingConfig.rates.forEach((rate) => {
            if (rate.carrier_code === previousCode) rate.carrier_code = value;
          });
        }, {
          required: true,
          pattern: "[a-z0-9]+(-[a-z0-9]+)*",
        }),
        shippingField("Nom affiché", carrier.name, (value) => { carrier.name = value; }, { required: true }),
        shippingToggle("Actif", carrier.active, (value) => { carrier.active = value; }),
      );
      const remove = document.createElement("button");
      remove.type = "button";
      remove.className = "danger";
      remove.textContent = "Supprimer";
      remove.addEventListener("click", () => {
        const [removed] = shippingConfig.carriers.splice(index, 1);
        shippingConfig.services = shippingConfig.services.filter((service) => service.carrier_code !== removed.code);
        shippingConfig.rates = shippingConfig.rates.filter((rate) => rate.carrier_code !== removed.code);
        renderShipping(shippingConfig);
      });
      card.append(remove);
      shippingCarriers.append(card);
    });

    shippingConfig.services.forEach((service, index) => {
      const card = document.createElement("article");
      card.className = "card";
      const carrierLabel = document.createElement("label");
      carrierLabel.textContent = "Transporteur";
      const carrierSelect = document.createElement("select");
      for (const carrier of shippingConfig.carriers) {
        const option = document.createElement("option");
        option.value = carrier.code;
        option.textContent = carrier.name || carrier.code;
        option.selected = carrier.code === service.carrier_code;
        carrierSelect.append(option);
      }
      carrierSelect.addEventListener("change", () => {
        const previousCarrierCode = service.carrier_code;
        service.carrier_code = carrierSelect.value;
        shippingConfig.rates.forEach((rate) => {
          if (rate.carrier_code === previousCarrierCode && rate.service_code === service.code) {
            rate.carrier_code = service.carrier_code;
          }
        });
      });
      carrierLabel.append(carrierSelect);
      card.append(
        carrierLabel,
        shippingField("Code du service", service.code, (value) => {
          const previousCode = service.code;
          service.code = value;
          shippingConfig.rates.forEach((rate) => {
            if (rate.carrier_code === service.carrier_code && rate.service_code === previousCode) rate.service_code = value;
          });
        }, { required: true, pattern: "[a-z0-9]+(-[a-z0-9]+)*" }),
        shippingField("Nom affiché", service.name, (value) => { service.name = value; }, { required: true }),
        shippingToggle("Actif", service.active, (value) => { service.active = value; }),
      );
      const remove = document.createElement("button");
      remove.type = "button";
      remove.className = "danger";
      remove.textContent = "Supprimer";
      remove.addEventListener("click", () => {
        const [removed] = shippingConfig.services.splice(index, 1);
        shippingConfig.rates = shippingConfig.rates.filter((rate) =>
          rate.carrier_code !== removed.carrier_code || rate.service_code !== removed.code);
        renderShipping(shippingConfig);
      });
      card.append(remove);
      shippingServices.append(card);
    });

    shippingConfig.zones.forEach((zone, index) => {
      const card = document.createElement("article");
      card.className = "card";
      card.append(
        shippingField("Code", zone.code, (value) => {
          const previousCode = zone.code;
          zone.code = value;
          shippingConfig.rates.forEach((rate) => {
            if (rate.zone_code === previousCode) rate.zone_code = value;
          });
        }, {
          required: true,
          pattern: "[a-z0-9]+(-[a-z0-9]+)*",
        }),
        shippingField("Nom", zone.name, (value) => { zone.name = value; }, { required: true }),
        shippingField("Pays (codes ISO à deux lettres séparés par des virgules)", zone.country_codes.join(", "),
          (value) => { zone.country_codes = value.split(",").map((country) => country.trim()).filter(Boolean); },
          { required: true }),
        shippingToggle("Active", zone.active, (value) => { zone.active = value; }),
      );
      const remove = document.createElement("button");
      remove.type = "button";
      remove.className = "danger";
      remove.textContent = "Supprimer";
      remove.addEventListener("click", () => {
        const [removed] = shippingConfig.zones.splice(index, 1);
        shippingConfig.rates = shippingConfig.rates.filter((rate) => rate.zone_code !== removed.code);
        renderShipping(shippingConfig);
      });
      card.append(remove);
      shippingZones.append(card);
    });

    shippingConfig.rates.forEach((rate, index) => {
      if (!rate.mode) rate.mode = "fixed";
      if (!Array.isArray(rate.rules)) rate.rules = [];
      if (typeof rate.ruleText !== "string") rate.ruleText = JSON.stringify(rate.rules, null, 2);
      const card = document.createElement("article");
      card.className = "card";
      const carrierLabel = document.createElement("label");
      carrierLabel.textContent = "Transporteur";
      const carrierSelect = document.createElement("select");
      for (const carrier of shippingConfig.carriers) {
        const option = document.createElement("option");
        option.value = carrier.code;
        option.textContent = carrier.name || carrier.code;
        option.selected = carrier.code === rate.carrier_code;
        carrierSelect.append(option);
      }
      carrierSelect.addEventListener("change", () => {
        rate.carrier_code = carrierSelect.value;
        const services = shippingConfig.services.filter((service) => service.carrier_code === rate.carrier_code);
        if (!services.some((service) => service.code === rate.service_code)) {
          rate.service_code = services[0]?.code || "";
        }
        renderShipping(shippingConfig);
      });
      carrierLabel.append(carrierSelect);
      const serviceLabel = document.createElement("label");
      serviceLabel.textContent = "Service de livraison";
      const serviceSelect = document.createElement("select");
      for (const service of shippingConfig.services.filter((entry) => entry.carrier_code === rate.carrier_code)) {
        const option = document.createElement("option");
        option.value = service.code;
        option.textContent = service.name || service.code;
        option.selected = service.code === rate.service_code;
        serviceSelect.append(option);
      }
      serviceSelect.addEventListener("change", () => { rate.service_code = serviceSelect.value; });
      serviceLabel.append(serviceSelect);
      const zoneLabel = document.createElement("label");
      zoneLabel.textContent = "Zone";
      const zoneSelect = document.createElement("select");
      for (const zone of shippingConfig.zones) {
        const option = document.createElement("option");
        option.value = zone.code;
        option.textContent = zone.name || zone.code;
        option.selected = zone.code === rate.zone_code;
        zoneSelect.append(option);
      }
      zoneSelect.addEventListener("change", () => { rate.zone_code = zoneSelect.value; });
      zoneLabel.append(zoneSelect);
      const modeLabel = document.createElement("label");
      modeLabel.textContent = "Calcul du tarif";
      const modeSelect = document.createElement("select");
      for (const [value, labelText] of [["fixed", "Forfait"], ["weight_value", "Poids × sous-total"]]) {
        const option = document.createElement("option");
        option.value = value;
        option.textContent = labelText;
        option.selected = rate.mode === value;
        modeSelect.append(option);
      }
      modeSelect.addEventListener("change", () => {
        rate.mode = modeSelect.value;
        renderShipping(shippingConfig);
      });
      modeLabel.append(modeSelect);
      const fields = [carrierLabel, serviceLabel, zoneLabel, modeLabel];
      if (rate.mode === "fixed") {
        fields.push(shippingField("Tarif forfaitaire", rate.amount, (value) => { rate.amount = value; }, {
          required: true,
          type: "number",
          min: "0",
          step: "0.0001",
        }));
      } else {
        const rulesLabel = document.createElement("label");
        rulesLabel.textContent = "Tranches poids × sous-total — JSON";
        const rulesEditor = document.createElement("textarea");
        rulesEditor.spellcheck = false;
        rulesEditor.value = rate.ruleText;
        rulesEditor.addEventListener("input", () => { rate.ruleText = rulesEditor.value; });
        rulesLabel.append(rulesEditor);
        const hint = document.createElement("p");
        hint.className = "muted";
        hint.textContent =
          'Poids en grammes (min inclus, max exclu ; max null = sans limite). Sous-total produits hors taxes et livraison (mêmes bornes). Exemple : [{"min_weight_grams":0,"max_weight_grams":1000,"min_subtotal":"0","max_subtotal":null,"amount":"5.90"}].';
        rulesLabel.append(hint);
        fields.push(rulesLabel);
      }
      card.append(
        ...fields,
        Object.assign(document.createElement("p"), {
          className: "muted",
          textContent: `Version ${rate.rate_version || "créée à l’enregistrement"}`,
        }),
        shippingField("Devise", rate.currency, (value) => { rate.currency = value.toUpperCase(); }, {
          required: true,
          pattern: "[A-Z]{3}",
        }),
        shippingToggle("Actif", rate.active, (value) => { rate.active = value; }),
      );
      const remove = document.createElement("button");
      remove.type = "button";
      remove.className = "danger";
      remove.textContent = "Supprimer";
      remove.addEventListener("click", () => {
        shippingConfig.rates.splice(index, 1);
        renderShipping(shippingConfig);
      });
      card.append(remove);
      shippingRates.append(card);
    });
    const shopID = currentIdentity?.shop_id;
    ensureShippingHistoryTargets().then(() => {
      if (shopID !== currentIdentity?.shop_id) return;
      renderShippingHistoryFilters();
      refreshShippingHistory();
    }).catch((error) => setStatus(error.message, true));
  }

  async function ensureShippingHistoryTargets() {
    const shopID = currentIdentity?.shop_id;
    if (shippingHistoryTargets !== null && shippingHistoryTargetsShopID === shopID) return;
    if (!shippingHistoryTargetsPromise || shippingHistoryTargetsPromiseShopID !== shopID) {
      const generation = shippingHistoryTargetsGeneration;
      shippingHistoryTargetsPromiseShopID = shopID;
      shippingHistoryTargetsPromise = request("/v1/admin/shipping/versions/targets").then((targets) => {
        if (generation === shippingHistoryTargetsGeneration && shopID === currentIdentity?.shop_id) {
          shippingHistoryTargets = targets;
          shippingHistoryTargetsShopID = shopID;
        }
      });
    }
    const pending = shippingHistoryTargetsPromise;
    try {
      await pending;
    } catch (error) {
      if (pending === shippingHistoryTargetsPromise) {
        shippingHistoryTargetsPromise = null;
        shippingHistoryTargetsPromiseShopID = null;
      }
      throw error;
    }
  }

  function renderShippingHistoryFilters() {
    shippingHistoryFilters.replaceChildren();
    const targets = shippingHistoryTargetsShopID === currentIdentity?.shop_id ? shippingHistoryTargets || [] : [];
    const carrierNames = new Map(shippingConfig.carriers.map((carrier) => [carrier.code, carrier.name || carrier.code]));
    const serviceNames = new Map(shippingConfig.services.map((service) => [
      `${service.carrier_code}\u0000${service.code}`,
      service.name || service.code,
    ]));
    const choicesFrom = (items, key, label) => [...new Map(items.map((item) => [item[key], [item[key], label(item)]])).values()];
    const choices = [
      {
        key: "carrierCode",
        label: "Transporteur",
        values: choicesFrom(
          [...shippingConfig.carriers, ...targets.map((target) => ({ code: target.carrier_code }))],
          "code",
          (carrier) => carrierNames.get(carrier.code) || carrier.code,
        ),
      },
      {
        key: "serviceCode",
        label: "Service",
        values: choicesFrom([
          ...shippingConfig.services.filter((service) => service.carrier_code === shippingHistoryFilter.carrierCode),
          ...targets
            .filter((target) => target.carrier_code === shippingHistoryFilter.carrierCode)
            .map((target) => ({ carrier_code: target.carrier_code, code: target.service_code })),
        ], "code", (service) => serviceNames.get(`${shippingHistoryFilter.carrierCode}\u0000${service.code}`) || service.code),
      },
      {
        key: "zoneCode",
        label: "Zone",
        values: choicesFrom([
          ...shippingConfig.zones,
          ...targets
            .filter((target) =>
              target.carrier_code === shippingHistoryFilter.carrierCode &&
              target.service_code === shippingHistoryFilter.serviceCode)
            .map((target) => ({ code: target.zone_code })),
        ], "code", (zone) => zone.name || zone.code),
      },
    ];
    for (const choice of choices) {
      const label = document.createElement("label");
      label.textContent = choice.label;
      const select = document.createElement("select");
      select.disabled = choice.values.length === 0;
      for (const [value, text] of choice.values) {
        const option = document.createElement("option");
        option.value = value;
        option.textContent = text;
        select.append(option);
      }
      if (!choice.values.some(([value]) => value === shippingHistoryFilter[choice.key])) {
        shippingHistoryFilter[choice.key] = choice.values[0]?.[0] || "";
      }
      select.value = shippingHistoryFilter[choice.key];
      select.addEventListener("change", () => {
        shippingHistoryFilter[choice.key] = select.value;
        if (choice.key === "carrierCode") {
          shippingHistoryFilter.serviceCode = "";
        }
        shippingHistoryOffset = 0;
        shippingHistoryComparison.hidden = true;
        renderShippingHistoryFilters();
        refreshShippingHistory();
      });
      label.append(select);
      shippingHistoryFilters.append(label);
    }
  }

  async function refreshShippingHistory() {
    const requestNumber = ++shippingHistoryRequest;
    shippingHistoryList.replaceChildren();
    shippingHistoryPagination.hidden = true;
    if (!shippingHistoryFilter.carrierCode || !shippingHistoryFilter.serviceCode || !shippingHistoryFilter.zoneCode) {
      shippingHistoryList.textContent = "Ajoutez un transporteur, un service et une zone pour consulter l’historique.";
      return;
    }
    const query = new URLSearchParams({
      carrier_code: shippingHistoryFilter.carrierCode,
      service_code: shippingHistoryFilter.serviceCode,
      zone_code: shippingHistoryFilter.zoneCode,
      limit: String(shippingHistoryLimit),
      offset: String(shippingHistoryOffset),
    });
    try {
      const versions = await request(`/v1/admin/shipping/versions?${query}`);
      if (requestNumber !== shippingHistoryRequest) return;
      if (!versions.length) {
        shippingHistoryList.textContent = "Aucune version enregistrée pour ce service et cette zone.";
        return;
      }
      const currentRate = shippingConfig.rates.find((rate) =>
        rate.carrier_code === shippingHistoryFilter.carrierCode &&
        rate.service_code === shippingHistoryFilter.serviceCode &&
        rate.zone_code === shippingHistoryFilter.zoneCode);
      versions.forEach((version) => {
        const card = document.createElement("article");
        card.className = "card";
        const heading = document.createElement("h4");
        heading.textContent = `${version.service_name} · ${version.amount} ${version.currency}`;
        const metadata = document.createElement("p");
        metadata.className = "muted";
        metadata.textContent = `Version ${version.rate_version} · ${new Date(version.created_at).toLocaleString()} · ${version.mode}${version.active ? "" : " · inactive"}`;
        const actions = document.createElement("div");
        actions.className = "actions";
        const compare = document.createElement("button");
        compare.type = "button";
        compare.className = "secondary";
        compare.textContent = "Comparer au tarif actif";
        compare.disabled = !currentRate;
        compare.addEventListener("click", () => {
          shippingHistoryComparison.replaceChildren();
          const title = document.createElement("h4");
          title.textContent = "Comparaison avec le tarif actif";
          const selectedTitle = document.createElement("h5");
          selectedTitle.textContent = `Version sélectionnée · ${version.rate_version}`;
          const selected = document.createElement("pre");
          selected.textContent = JSON.stringify(version, null, 2);
          const currentTitle = document.createElement("h5");
          currentTitle.textContent = `Version active · ${currentRate?.rate_version || "aucune"}`;
          const current = document.createElement("pre");
          current.textContent = currentRate
            ? JSON.stringify({
              rate_version: currentRate.rate_version,
              mode: currentRate.mode,
              amount: currentRate.amount,
              currency: currentRate.currency,
              active: currentRate.active,
              rules: currentRate.rules || [],
            }, null, 2)
            : "Aucun tarif actuellement configuré.";
          shippingHistoryComparison.append(title, selectedTitle, selected, currentTitle, current);
          shippingHistoryComparison.hidden = false;
        });
        const restore = document.createElement("button");
        restore.type = "button";
        restore.textContent = "Restaurer cette version";
        restore.addEventListener("click", async () => {
          if (!window.confirm("Ce tarif sera publié comme une nouvelle version active. Les commandes existantes ne seront pas modifiées. Continuer ?")) return;
          restore.disabled = true;
          try {
            await request(`/v1/admin/shipping/versions/${encodeURIComponent(version.rate_version)}/restore`, {
              method: "POST",
            });
            renderShipping(await request("/v1/admin/shipping"));
            setStatus("Tarif restauré dans une nouvelle version.");
          } catch (error) {
            setStatus(error.message, true);
          } finally {
            restore.disabled = false;
          }
        });
        actions.append(compare, restore);
        card.append(heading, metadata, actions);
        shippingHistoryList.append(card);
      });
      shippingHistoryPagination.hidden = false;
      document.querySelector("#shipping-history-previous").disabled = shippingHistoryOffset === 0;
      document.querySelector("#shipping-history-next").disabled = versions.length < shippingHistoryLimit;
    } catch (error) {
      if (requestNumber === shippingHistoryRequest) {
        shippingHistoryList.textContent = error.message;
        setStatus(error.message, true);
      }
    }
  }

  function editProduct(product) {
    productForm.elements.id.value = product.id;
    productForm.elements.slug.value = product.slug;
    productForm.elements.status.value = product.status;
    productForm.elements.translations.value = JSON.stringify(product.translations, null, 2);
    productForm.elements.media.value = JSON.stringify((product.media || []).map((media) => ({
      uri: media.uri,
      mime_type: media.mime_type,
      alt: media.alt,
    })), null, 2);
    productForm.elements.variants.value = JSON.stringify(product.variants.map((variant) => ({
      sku: variant.sku,
      options: variant.options || {},
      prices: variant.prices || [],
      inventory: variant.inventory || { quantity: 0, track: true },
    })), null, 2);
    for (const checkbox of productForm.querySelectorAll('input[name="category_ids"]')) {
      checkbox.checked = product.category_ids.includes(checkbox.value);
    }
    document.querySelector("#product-form-title").textContent = "Modifier le produit";
    document.querySelector("#cancel-product-edit").hidden = false;
    productForm.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  function editCategory(category) {
    editingCategory = category;
    categoryForm.elements.id.value = category.id;
    categoryForm.elements.slug.value = category.slug;
    categoryForm.elements.translations.value = JSON.stringify(category.translations, null, 2);
    document.querySelector("#category-form-title").textContent = "Modifier la catégorie";
    document.querySelector("#cancel-category-edit").hidden = false;
    categoryForm.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  function editStaff(member) {
    staffForm.elements.id.value = member.id;
    staffForm.elements.email.value = member.email;
    staffForm.elements.email.readOnly = true;
    staffForm.elements.name.value = member.name;
    staffForm.elements.role.value = member.role;
    staffForm.elements.password.value = "";
    document.querySelector("#staff-form-title").textContent = "Modifier un membre";
    document.querySelector("#cancel-staff-edit").hidden = false;
    staffForm.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  function resetProductForm() {
    productForm.reset();
    productForm.elements.id.value = "";
    productForm.elements.status.value = "draft";
    productForm.elements.translations.value = '[\n  { "locale": "fr-FR", "name": "Nom du produit", "description": "" }\n]';
    productForm.elements.media.value = "[]";
    productForm.elements.variants.value = '[\n  {\n    "sku": "ARTICLE-001",\n    "options": {},\n    "weight_grams": null,\n    "prices": [{ "currency": "EUR", "amount": "12.0000" }],\n    "inventory": { "quantity": 10, "track": true }\n  }\n]';
    for (const checkbox of productForm.querySelectorAll('input[name="category_ids"]')) checkbox.checked = false;
    document.querySelector("#product-form-title").textContent = "Créer un produit";
    document.querySelector("#cancel-product-edit").hidden = true;
  }

  function resetCategoryForm() {
    categoryForm.reset();
    categoryForm.elements.id.value = "";
    categoryForm.elements.translations.value = '[\n  { "locale": "fr-FR", "name": "Nom de la catégorie", "description": "" }\n]';
    editingCategory = null;
    document.querySelector("#category-form-title").textContent = "Créer une catégorie";
    document.querySelector("#cancel-category-edit").hidden = true;
  }

  function resetStaffForm() {
    staffForm.reset();
    staffForm.elements.id.value = "";
    staffForm.elements.email.readOnly = false;
    staffForm.elements.role.value = "editor";
    document.querySelector("#staff-form-title").textContent = "Ajouter un membre";
    document.querySelector("#cancel-staff-edit").hidden = true;
  }

  async function refresh() {
    try {
      const [products, loadedCategories] = await Promise.all([
        request("/v1/admin/products"),
        request("/v1/admin/categories"),
      ]);
      categories = loadedCategories;
      renderCategories();
      renderProducts(products);
      if (currentIdentity?.role === "owner") {
        renderStaff(await request("/v1/admin/staff"));
      }
      if (["owner", "admin", "support"].includes(currentIdentity?.role)) {
        renderOrders(await request("/v1/admin/orders"));
      }
      if (["owner", "admin"].includes(currentIdentity?.role)) {
        renderShipping(await request("/v1/admin/shipping"));
      }
      setStatus(`${products.length} produit(s), ${categories.length} catégorie(s).`);
    } catch (error) {
      setStatus(error.message, true);
    }
  }

  document.querySelector("#login-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const loginForm = event.currentTarget;
    const form = new FormData(loginForm);
    const button = loginForm.querySelector('button[type="submit"]');
    button.disabled = true;
    try {
      const result = await request("/v1/admin/login", {
        method: "POST",
        body: JSON.stringify({ email: form.get("email"), password: form.get("password") }),
      });
      token = result.token;
      sessionStorage.setItem(tokenKey, token);
      showDashboard(result);
      await refresh();
    } catch (error) {
      setStatus(error.message, true);
      loginPanel.querySelector(".status")?.remove();
      const message = document.createElement("p");
      message.className = "status";
      message.dataset.kind = "error";
      message.textContent = error.message;
      loginForm.append(message);
    } finally {
      button.disabled = false;
      loginForm.elements.password.value = "";
    }
  });

  productForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = new FormData(productForm);
    let translations;
    let media;
    let variants;
    try {
      translations = JSON.parse(form.get("translations"));
      media = JSON.parse(form.get("media"));
      variants = JSON.parse(form.get("variants"));
      if (!Array.isArray(translations) || !Array.isArray(media) || !Array.isArray(variants)) throw new Error();
    } catch {
      setStatus("Les traductions, images et variantes doivent être des tableaux JSON valides.", true);
      return;
    }
    const product = {
      slug: form.get("slug"),
      status: form.get("status"),
      translations,
      media,
      variants,
      category_ids: [...productForm.querySelectorAll('input[name="category_ids"]:checked')].map((input) => input.value),
    };
    try {
      const id = form.get("id");
      await request(id ? `/v1/admin/products/${encodeURIComponent(id)}` : "/v1/admin/products", {
        method: id ? "PUT" : "POST",
        body: JSON.stringify(product),
      });
      resetProductForm();
      setStatus("Produit enregistré.");
      await refresh();
    } catch (error) {
      setStatus(error.message, true);
    }
  });

  categoryForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = new FormData(categoryForm);
    let translations;
    try {
      translations = JSON.parse(form.get("translations"));
      if (!Array.isArray(translations)) throw new Error();
    } catch {
      setStatus("Les traductions doivent être un tableau JSON valide.", true);
      return;
    }
    const category = { slug: form.get("slug"), translations };
    const id = form.get("id");
    try {
      await request(id ? `/v1/admin/categories/${encodeURIComponent(id)}` : "/v1/admin/categories", {
        method: id ? "PUT" : "POST",
        body: JSON.stringify(category),
      });
      resetCategoryForm();
      setStatus("Catégorie enregistrée.");
      await refresh();
    } catch (error) {
      setStatus(error.message, true);
    }
  });

  staffForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = new FormData(staffForm);
    const id = form.get("id");
    const member = {
      email: form.get("email"),
      name: form.get("name"),
      role: form.get("role"),
      password: form.get("password"),
    };
    try {
      await request(id ? `/v1/admin/staff/${encodeURIComponent(id)}` : "/v1/admin/staff", {
        method: id ? "PUT" : "POST",
        body: JSON.stringify(member),
      });
      resetStaffForm();
      setStatus("Membre enregistré.");
      await refresh();
    } catch (error) {
      setStatus(error.message, true);
    } finally {
      staffForm.elements.password.value = "";
    }
  });

  document.querySelector("#cancel-product-edit").addEventListener("click", resetProductForm);
  document.querySelector("#cancel-category-edit").addEventListener("click", resetCategoryForm);
  document.querySelector("#cancel-staff-edit").addEventListener("click", resetStaffForm);
  document.querySelector("#shipping-history-previous").addEventListener("click", () => {
    shippingHistoryOffset = Math.max(0, shippingHistoryOffset - shippingHistoryLimit);
    refreshShippingHistory();
  });
  document.querySelector("#shipping-history-next").addEventListener("click", () => {
    shippingHistoryOffset += shippingHistoryLimit;
    refreshShippingHistory();
  });
  document.querySelector("#add-shipping-carrier").addEventListener("click", () => {
    shippingConfig.carriers.push({ code: "", name: "", active: false });
    renderShipping(shippingConfig);
  });
  document.querySelector("#add-shipping-service").addEventListener("click", () => {
    shippingConfig.services.push({
      carrier_code: shippingConfig.carriers[0]?.code || "",
      code: "",
      name: "",
      active: true,
    });
    renderShipping(shippingConfig);
  });
  document.querySelector("#add-shipping-zone").addEventListener("click", () => {
    shippingConfig.zones.push({ code: "", name: "", country_codes: [], active: true });
    renderShipping(shippingConfig);
  });
  document.querySelector("#add-shipping-rate").addEventListener("click", () => {
    shippingConfig.rates.push({
      carrier_code: shippingConfig.carriers[0]?.code || "",
      service_code: shippingConfig.services.find((service) => service.carrier_code === shippingConfig.carriers[0]?.code)?.code || "",
      zone_code: shippingConfig.zones[0]?.code || "",
      mode: "fixed",
      amount: "0.00",
      currency: "EUR",
      active: true,
    });
    renderShipping(shippingConfig);
  });
  saveShippingButton.addEventListener("click", async () => {
    saveShippingButton.disabled = true;
    try {
      const saved = await request("/v1/admin/shipping", {
        method: "PUT",
        body: JSON.stringify({
          ...shippingConfig,
          rates: shippingConfig.rates.map((rate) => {
            const { ruleText, ...savedRate } = rate;
            if (savedRate.mode === "weight_value") {
              savedRate.rules = JSON.parse(ruleText);
              savedRate.amount = "";
            } else {
              savedRate.rules = [];
            }
            return savedRate;
          }),
        }),
      });
      renderShipping(saved);
      setStatus("Configuration transport enregistrée.");
    } catch (error) {
      setStatus(error.message, true);
    } finally {
      saveShippingButton.disabled = false;
    }
  });
  logoutButton.addEventListener("click", async () => {
    try {
      await request("/v1/admin/logout", { method: "POST" });
    } catch (error) {
      setStatus(error.message, true);
    } finally {
      clearSession();
    }
  });

  try {
    token = sessionStorage.getItem(tokenKey);
    if (token) {
      request("/v1/admin/me")
        .then(async (identity) => {
          showDashboard(identity);
          await refresh();
        })
        .catch((error) => setStatus(error.message, true));
    }
  } catch {
    setStatus("Le stockage de session est indisponible.", true);
  }
})();
