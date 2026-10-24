(() => {
  const tokenKey = "euromni.staff.token";
  const loginPanel = document.querySelector("#login-panel");
  const dashboard = document.querySelector("#dashboard");
  const adminTabs = [...document.querySelectorAll("[data-admin-tab]")];
  const adminTabPanels = [...document.querySelectorAll("[data-admin-panel]")];
  const status = document.querySelector("#status");
  const logoutButton = document.querySelector("#logout");
  const productForm = document.querySelector("#product-form");
  const moduleSettingsPanel = document.querySelector("#module-settings-panel");
  const moduleInventoryStatus = document.querySelector("#module-inventory-status");
  const moduleInventory = document.querySelector("#module-inventory");
  const buildPackageCandidatesStatus = document.querySelector("#build-package-candidates-status");
  const buildPackageCandidates = document.querySelector("#build-package-candidates");
  const buildPackageCandidateDetail = document.querySelector("#build-package-candidate-detail");
  const buildPackageCandidatesPrevious = document.querySelector("#build-package-candidates-previous");
  const buildPackageCandidatesNext = document.querySelector("#build-package-candidates-next");
  const moduleSettingsStatus = document.querySelector("#module-settings-status");
  const newProductsSettingsForm = document.querySelector("#new-products-settings-form");
  const categoryForm = document.querySelector("#category-form");
  const brandForm = document.querySelector("#brand-form");
  const supplierForm = document.querySelector("#supplier-form");
  const staffForm = document.querySelector("#staff-form");
  const productList = document.querySelector("#products");
  const categoryList = document.querySelector("#categories");
  const categoryOptions = document.querySelector("#category-options");
  const brandList = document.querySelector("#brands");
  const brandSelect = document.querySelector("#brand-select");
  const supplierList = document.querySelector("#suppliers");
  const productSupplierOptions = document.querySelector("#product-supplier-options");
  const staffList = document.querySelector("#staff-members");
  const orderList = document.querySelector("#orders");
  const orderDetail = document.querySelector("#order-detail");
  const emailsPanel = document.querySelector("#emails-panel");
  const emailsStatus = document.querySelector("#emails-status");
  const emailsList = document.querySelector("#emails-list");
  const emailDetail = document.querySelector("#email-detail");
  const emailsPrevious = document.querySelector("#emails-previous");
  const emailsNext = document.querySelector("#emails-next");
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
  let brands = [];
  let suppliers = [];
  let editingCategory = null;
  let editingBrand = null;
  let editingSupplier = null;
  let currentIdentity = null;
  let buildPackageCandidatesOffset = 0;
  let buildPackageCandidatesRequest = 0;
  let buildPackageCandidateDetailRequest = 0;
  let emailOffset = 0;
  let emailRequest = 0;
  let emailDetailRequest = 0;
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
      const error = new Error(payload.error || `Erreur HTTP ${response.status}`);
      error.status = response.status;
      throw error;
    }
    return payload.data;
  }

  function activateAdminTab(name, updateHistory = true) {
    const selectedTab = adminTabs.find((tab) => tab.dataset.adminTab === name && !tab.hidden) ||
      adminTabs.find((tab) => !tab.hidden);
    if (!selectedTab) return;

    for (const tab of adminTabs) {
      const selected = tab === selectedTab;
      tab.setAttribute("aria-selected", String(selected));
      tab.tabIndex = selected ? 0 : -1;
    }
    for (const panel of adminTabPanels) {
      panel.hidden = panel.dataset.adminPanel !== selectedTab.dataset.adminTab;
    }

    const hash = `#${selectedTab.dataset.adminTab}`;
    if (updateHistory && window.location.hash !== hash) {
      window.history.pushState(null, "", hash);
    } else if (!updateHistory && window.location.hash !== hash) {
      window.history.replaceState(null, "", hash);
    }
    if (selectedTab.dataset.adminTab === "emails" && !dashboard.hidden) {
      loadAdminEmails(emailOffset);
    }
  }

  function activateAdminTabFromHash() {
    if (!dashboard.hidden) activateAdminTab(window.location.hash.slice(1), false);
  }

  function showDashboard(identity) {
    if (currentIdentity?.shop_id !== identity.shop_id) {
      shippingHistoryTargets = null;
      shippingHistoryTargetsPromise = null;
      shippingHistoryTargetsPromiseShopID = null;
      shippingHistoryTargetsShopID = null;
      shippingHistoryTargetsGeneration += 1;
      shippingHistoryRequest += 1;
      emailRequest += 1;
      emailDetailRequest += 1;
      buildPackageCandidatesRequest += 1;
      buildPackageCandidateDetailRequest += 1;
      buildPackageCandidatesOffset = 0;
      buildPackageCandidates.replaceChildren();
      buildPackageCandidateDetail.replaceChildren();
      buildPackageCandidateDetail.hidden = true;
      emailOffset = 0;
    }
    currentIdentity = identity;
    loginPanel.hidden = true;
    dashboard.hidden = false;
    logoutButton.hidden = false;
    const isOwner = identity.role === "owner";
    const canManageShipping = ["owner", "admin"].includes(identity.role);
    const canViewOrders = ["owner", "admin", "support"].includes(identity.role);
    document.querySelector("#staff-panel").hidden = !isOwner;
    document.querySelector("#orders-panel").hidden = !canViewOrders;
    shippingPanel.hidden = !canManageShipping;
    moduleSettingsPanel.hidden = !canManageShipping;
    emailsPanel.hidden = !["owner", "admin"].includes(identity.role);
    document.querySelector("#tab-equipe").hidden = !isOwner;
    document.querySelector("#tab-commandes").hidden = !canViewOrders;
    document.querySelector("#tab-livraison").hidden = !canManageShipping;
    document.querySelector("#tab-modules").hidden = !canManageShipping;
    document.querySelector("#tab-emails").hidden = !["owner", "admin"].includes(identity.role);
    document.querySelector('[data-admin-open-tab="commandes"]').closest("li").hidden = !canViewOrders;
    document.querySelector('[data-admin-open-tab="livraison"]').closest("li").hidden = !canManageShipping;
    document.querySelector("#identity").textContent =
      `${identity.name || identity.email} · ${identity.email} · rôle ${identity.role}`;
    activateAdminTab(window.location.hash.slice(1), false);
  }

  function clearSession() {
    token = null;
    shippingHistoryTargets = null;
    shippingHistoryTargetsPromise = null;
    shippingHistoryTargetsPromiseShopID = null;
    shippingHistoryTargetsShopID = null;
    shippingHistoryTargetsGeneration += 1;
    shippingHistoryRequest += 1;
    emailRequest += 1;
    emailDetailRequest += 1;
    buildPackageCandidatesRequest += 1;
    buildPackageCandidateDetailRequest += 1;
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

  function orderStatusLabel(status, shipmentStatus) {
    if (shipmentStatus === "delivered") return "Livrée";
    if (shipmentStatus === "ready") return "Préparée";
    if (shipmentStatus === "shipped" || status === "fulfilled") return "Expédiée";
    return ({
      pending: "En attente de paiement",
      awaiting_payment: "Paiement attendu",
      paid: "Commande reçue (payée)",
      fulfilling: "En cours de traitement",
      cancelled: "Annulée",
      refunded: "Remboursée",
    })[status] || status;
  }

  function shipmentStatusLabel(status) {
    return ({
      pending: "En attente",
      ready: "Préparée",
      shipped: "Expédiée",
      delivered: "Livrée",
      returned: "Retournée",
    })[status] || status;
  }

  function emailEventLabel(eventType) {
    return ({
      order_confirmation: "Confirmation de commande",
      order_status: "Mise à jour de commande",
    })[eventType] || eventType;
  }

  function emailStatusLabel(emailStatus) {
    return ({
      pending: "En attente",
      processing: "En cours d’envoi",
      sent: "Envoyé",
      failed: "Échec définitif",
    })[emailStatus] || emailStatus;
  }

  function formatAdminDate(value) {
    if (!value) return "—";
    const date = new Date(value);
    return Number.isNaN(date.getTime())
      ? "—"
      : new Intl.DateTimeFormat("fr-FR", { dateStyle: "medium", timeStyle: "short" }).format(date);
  }

  async function loadAdminEmails(offset = 0) {
    const requestID = ++emailRequest;
    emailsStatus.textContent = "Chargement de l’historique…";
    emailsStatus.dataset.kind = "info";
    emailsPrevious.disabled = offset <= 0;
    emailsNext.disabled = true;
    try {
      const page = await request(`/v1/admin/emails?limit=50&offset=${offset}`);
      if (requestID !== emailRequest) return;
      emailOffset = page.offset;
      emailsList.replaceChildren();
      emailDetail.replaceChildren();
      emailDetail.hidden = true;
      if (page.emails.length === 0) {
        const empty = document.createElement("p");
        empty.className = "muted";
        empty.textContent = "Aucun courriel enregistré dans l’outbox de cette boutique.";
        emailsList.append(empty);
      }
      for (const email of page.emails) {
        const card = document.createElement("article");
        card.className = "card";
        const heading = document.createElement("h3");
        heading.textContent = `${emailEventLabel(email.event_type)} · ${email.order_number || "Commande inconnue"}`;
        const summary = document.createElement("p");
        summary.className = "muted";
        summary.textContent = `${email.recipient} · ${emailStatusLabel(email.status)} · ${email.attempts} tentative(s) · créé ${formatAdminDate(email.created_at)}`;
        const actions = document.createElement("div");
        actions.className = "actions";
        const view = document.createElement("button");
        view.type = "button";
        view.className = "secondary";
        view.textContent = "Consulter le contenu";
        view.addEventListener("click", () => showAdminEmailDetail(email.id));
        actions.append(view);
        card.append(heading, summary, actions);
        emailsList.append(card);
      }
      emailsPrevious.disabled = emailOffset <= 0;
      emailsNext.disabled = !page.has_more;
      emailsStatus.textContent = page.emails.length
        ? `${page.emails.length} courriel(s) affiché(s), à partir de la position ${emailOffset + 1}.`
        : "Fin de l’historique.";
    } catch (error) {
      if (requestID !== emailRequest) return;
      emailsList.replaceChildren();
      emailsStatus.textContent = error.message;
      emailsStatus.dataset.kind = "error";
    }
  }

  async function showAdminEmailDetail(emailID) {
    const requestID = ++emailDetailRequest;
    emailDetail.hidden = false;
    emailDetail.replaceChildren();
    const loading = document.createElement("p");
    loading.className = "muted";
    loading.textContent = "Chargement du courriel…";
    emailDetail.append(loading);
    try {
      const email = await request(`/v1/admin/emails/${encodeURIComponent(emailID)}`);
      if (requestID !== emailDetailRequest) return;
      emailDetail.replaceChildren();
      const heading = document.createElement("h2");
      heading.textContent = email.subject || emailEventLabel(email.event_type);
      const metadata = document.createElement("p");
      metadata.className = "muted";
      metadata.textContent = `À : ${email.recipient} · ${emailStatusLabel(email.status)} · ${email.attempts} tentative(s) · créé ${formatAdminDate(email.created_at)}${email.sent_at ? ` · envoyé ${formatAdminDate(email.sent_at)}` : ""}`;
      const order = document.createElement("p");
      order.textContent = `Commande : ${email.order_number || "inconnue"} (${email.order_id})`;
      const previewTitle = document.createElement("h3");
      previewTitle.textContent = "Aperçu texte";
      const body = document.createElement("pre");
      body.style.whiteSpace = "pre-wrap";
      body.style.overflowWrap = "anywhere";
      body.textContent = email.text_body || JSON.stringify(email.payload, null, 2);
      emailDetail.append(heading, metadata, order);
      if (email.last_error) {
        const failure = document.createElement("p");
        failure.dataset.kind = "error";
        failure.textContent = `Dernière erreur : ${email.last_error}`;
        emailDetail.append(failure);
      }
      emailDetail.append(previewTitle, body);
    } catch (error) {
      if (requestID !== emailDetailRequest) return;
      emailDetail.replaceChildren();
      const failure = document.createElement("p");
      failure.dataset.kind = "error";
      failure.textContent = error.message;
      emailDetail.append(failure);
    }
  }

  function orderLifecycleLabel(eventType) {
    return ({
      order_placed: "Commande créée (paiement en attente)",
      paid: "Paiement confirmé",
      processing: "En cours de traitement",
      prepared: "Commande préparée",
      shipped: "Commande expédiée",
      delivered: "Commande livrée",
    })[eventType] || eventType;
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
      metadata.textContent = `${product.slug} · ${product.status} · ${product.variants.length} variante(s)` +
        (product.brand ? ` · ${product.brand.name}` : "");
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

  function renderCatalogReferences() {
    brandList.replaceChildren();
    brandSelect.replaceChildren(new Option("Aucune marque", ""));
    for (const brand of brands) {
      brandSelect.append(new Option(brand.name, brand.id));
      const card = document.createElement("article");
      card.className = "card";
      const title = document.createElement("h3");
      title.textContent = brand.name;
      const slug = document.createElement("p");
      slug.className = "muted";
      slug.textContent = brand.slug;
      const edit = document.createElement("button");
      edit.type = "button";
      edit.className = "secondary";
      edit.textContent = "Modifier";
      edit.addEventListener("click", () => editBrand(brand));
      card.append(title, slug, edit);
      brandList.append(card);
    }

    supplierList.replaceChildren();
    productSupplierOptions.replaceChildren();
    for (const supplier of suppliers) {
      const checkboxLabel = document.createElement("label");
      checkboxLabel.className = "category-option";
      const checkbox = document.createElement("input");
      checkbox.type = "checkbox";
      checkbox.name = "supplier_ids";
      checkbox.value = supplier.id;
      checkbox.disabled = !supplier.active;
      const name = document.createElement("span");
      name.textContent = `${supplier.name} (${supplier.code})${supplier.active ? "" : " · inactif"}`;
      checkboxLabel.append(checkbox, name);
      productSupplierOptions.append(checkboxLabel);

      const card = document.createElement("article");
      card.className = "card";
      const title = document.createElement("h3");
      title.textContent = supplier.name;
      const metadata = document.createElement("p");
      metadata.className = "muted";
      metadata.textContent = `${supplier.code} · ${supplier.active ? "actif" : "inactif"}`;
      const edit = document.createElement("button");
      edit.type = "button";
      edit.className = "secondary";
      edit.textContent = "Modifier";
      edit.addEventListener("click", () => editSupplier(supplier));
      card.append(title, metadata, edit);
      supplierList.append(card);
    }
  }

  function editBrand(brand) {
    editingBrand = brand;
    brandForm.elements.id.value = brand.id;
    brandForm.elements.slug.value = brand.slug;
    brandForm.elements.name.value = brand.name;
    document.querySelector("#brand-form-title").textContent = "Modifier une marque";
    document.querySelector("#cancel-brand-edit").hidden = false;
    brandForm.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  function editSupplier(supplier) {
    editingSupplier = supplier;
    supplierForm.elements.id.value = supplier.id;
    supplierForm.elements.code.value = supplier.code;
    supplierForm.elements.name.value = supplier.name;
    supplierForm.elements.active.checked = supplier.active;
    document.querySelector("#supplier-form-title").textContent = "Modifier un fournisseur";
    document.querySelector("#cancel-supplier-edit").hidden = false;
    supplierForm.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  function resetBrandForm() {
    brandForm.reset();
    brandForm.elements.id.value = "";
    editingBrand = null;
    document.querySelector("#brand-form-title").textContent = "Créer une marque";
    document.querySelector("#cancel-brand-edit").hidden = true;
  }

  function resetSupplierForm() {
    supplierForm.reset();
    supplierForm.elements.id.value = "";
    supplierForm.elements.active.checked = true;
    editingSupplier = null;
    document.querySelector("#supplier-form-title").textContent = "Créer un fournisseur";
    document.querySelector("#cancel-supplier-edit").hidden = true;
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
    heading.textContent = `Commande ${order.number} · ${orderStatusLabel(order.status, order.shipment_status)}`;
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
      if (item.personalizations?.length) {
        row.append(document.createElement("br"));
        const details = document.createElement("small");
        details.textContent = item.personalizations
          .map((selection) => `${selection.label} : ${selection.choice_label}`)
          .join(" · ");
        row.append(details);
      }
      items.append(row);
    }
    const address = document.createElement("p");
    address.textContent = `${order.shipping_address.line1 || ""} · ${order.shipping_address.postal_code || ""} ${order.shipping_address.city || ""} · ${order.shipping_address.country || ""}`;
    const timelineHeading = document.createElement("h4");
    timelineHeading.textContent = "Suivi de la commande";
    const timeline = document.createElement("ol");
    for (const event of order.lifecycle_events || []) {
      const entry = document.createElement("li");
      const occurredAt = new Date(event.occurred_at);
      const time = Number.isNaN(occurredAt.getTime())
        ? ""
        : ` · ${new Intl.DateTimeFormat("fr-FR", { dateStyle: "medium", timeStyle: "short" }).format(occurredAt)}`;
      const tracking = event.tracking ? ` · Suivi ${event.tracking}` : "";
      entry.textContent = `${orderLifecycleLabel(event.event_type)}${time}${tracking}`;
      timeline.append(entry);
    }
    orderDetail.append(total, items, address, timelineHeading);
    if (timeline.childElementCount) {
      orderDetail.append(timeline);
    } else {
      const emptyTimeline = document.createElement("p");
      emptyTimeline.className = "muted";
      emptyTimeline.textContent = "Aucune étape enregistrée pour cette commande.";
      orderDetail.append(emptyTimeline);
    }
  }

  function showOrderActionForm(card, options) {
    card.querySelector(".order-action-form")?.remove();

    const form = document.createElement("form");
    form.className = "order-action-form";
    const description = document.createElement("p");
    description.textContent = options.description;
    const label = document.createElement("label");
    label.append(document.createTextNode(options.label));
    const input = document.createElement("input");
    input.type = "text";
    input.maxLength = 512;
    input.autocomplete = "off";
    input.placeholder = "Facultatif";
    label.append(input);

    const buttons = document.createElement("div");
    buttons.className = "actions";
    const submit = document.createElement("button");
    submit.type = "submit";
    submit.textContent = options.submitLabel;
    const cancel = document.createElement("button");
    cancel.type = "button";
    cancel.className = "secondary";
    cancel.textContent = "Annuler";
    cancel.addEventListener("click", () => form.remove());
    buttons.append(submit, cancel);
    form.append(description, label, buttons);

    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      submit.disabled = true;
      cancel.disabled = true;
      try {
        await options.onSubmit(input.value.trim());
        form.remove();
      } catch (error) {
        submit.disabled = false;
        cancel.disabled = false;
        setStatus(error.message, true);
      }
    });

    card.append(form);
    input.focus();
  }

  function renderOrders(orders) {
    orderList.replaceChildren();
    orderDetail.hidden = true;
    for (const order of orders) {
      const card = document.createElement("article");
      card.className = "card";
      const title = document.createElement("h3");
      title.textContent = `${order.number} · ${orderStatusLabel(order.status, order.shipment_status)}`;
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
          showOrderActionForm(card, {
            description: `Confirmez que le paiement de ${order.grand_total} ${order.currency} pour ${order.number} a bien été reçu. Cette action n’effectue aucun débit.`,
            label: "Référence du virement ou reçu",
            submitLabel: "Confirmer le paiement reçu",
            onSubmit: async (providerReference) => {
              await request(`/v1/admin/orders/${encodeURIComponent(order.id)}/payments/manual`, {
                method: "POST",
                body: JSON.stringify({ provider_reference: providerReference }),
              });
              await refresh("Paiement manuel enregistré.");
            },
          });
        });
        actions.append(payment);
      }
      if (["owner", "admin"].includes(currentIdentity?.role)) {
        let nextEvent = "";
        let actionLabel = "";
        let successMessage = "";
        if (order.status === "paid") {
          nextEvent = "processing";
          actionLabel = "Commencer le traitement";
          successMessage = "Commande passée en cours de traitement.";
        } else if (order.status === "fulfilling" && order.shipment_status !== "ready") {
          nextEvent = "prepared";
          actionLabel = "Marquer comme préparée";
          successMessage = "Commande marquée comme préparée.";
        } else if (order.status === "fulfilling" && order.shipment_status === "ready") {
          nextEvent = "shipped";
          actionLabel = "Marquer comme expédiée";
          successMessage = "Commande marquée comme expédiée.";
        } else if (order.status === "fulfilled" && order.shipment_status === "shipped") {
          nextEvent = "delivered";
          actionLabel = "Confirmer la livraison";
          successMessage = "Commande marquée comme livrée.";
        }
        if (nextEvent) {
          const lifecycle = document.createElement("button");
          lifecycle.type = "button";
          lifecycle.textContent = actionLabel;
          lifecycle.addEventListener("click", async () => {
            if (nextEvent === "shipped") {
              showOrderActionForm(card, {
                description: `Confirmez la remise de la commande ${order.number} au transporteur.`,
                label: "Numéro de suivi",
                submitLabel: "Confirmer l’expédition",
                onSubmit: async (tracking) => {
                  await request(`/v1/admin/orders/${encodeURIComponent(order.id)}/lifecycle`, {
                    method: "POST",
                    body: JSON.stringify({ event_type: nextEvent, tracking }),
                  });
                  await refresh(successMessage);
                },
              });
              return;
            }
            try {
              await request(`/v1/admin/orders/${encodeURIComponent(order.id)}/lifecycle`, {
                method: "POST",
                body: JSON.stringify({ event_type: nextEvent }),
              });
              await refresh(successMessage);
            } catch (error) {
              setStatus(error.message, true);
            }
          });
          actions.append(lifecycle);
        }
        if (currentIdentity?.development_mode &&
            ["pending", "paid", "fulfilling", "fulfilled"].includes(order.status) &&
            order.shipment_status !== "delivered") {
          const demo = document.createElement("button");
          demo.type = "button";
          demo.className = "secondary";
          demo.textContent = "Simuler le parcours complet (local)";
          demo.addEventListener("click", async () => {
            try {
              await request(`/v1/admin/orders/${encodeURIComponent(order.id)}/lifecycle/demo`, {
                method: "POST",
                body: JSON.stringify({}),
              });
              await refresh("Parcours local terminé; les notifications sont dans l’outbox.");
            } catch (error) {
              setStatus(error.message, true);
            }
          });
          actions.append(demo);
        }
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
    productForm.elements.brand_id.value = product.brand?.id || "";
    productForm.elements.personalization_options.value = JSON.stringify(product.personalization_options || [], null, 2);
    productForm.elements.translations.value = JSON.stringify(product.translations, null, 2);
    productForm.elements.media.value = JSON.stringify((product.media || []).map((media) => ({
      uri: media.uri,
      mime_type: media.mime_type,
      alt: media.alt,
    })), null, 2);
    productForm.elements.variants.value = JSON.stringify(product.variants.map((variant) => ({
      sku: variant.sku,
      options: variant.options || {},
      weight_grams: variant.weight_grams ?? null,
      prices: variant.prices || [],
      inventory: variant.inventory || { quantity: 0, track: true },
      suppliers: (variant.suppliers || []).map((supplier) => ({
        supplier_id: supplier.supplier_id,
        supplier_sku: supplier.supplier_sku,
      })),
    })), null, 2);
    const supplierIDs = new Set(product.supplier_ids || []);
    for (const checkbox of productSupplierOptions.querySelectorAll('input[name="supplier_ids"]')) {
      checkbox.checked = supplierIDs.has(checkbox.value);
    }
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
    productForm.elements.brand_id.value = "";
    productForm.elements.personalization_options.value = "[]";
    productForm.elements.variants.value = '[\n  {\n    "sku": "ARTICLE-001",\n    "options": {},\n    "weight_grams": null,\n    "prices": [{ "currency": "EUR", "amount": "12.0000" }],\n    "inventory": { "quantity": 10, "track": true },\n    "suppliers": []\n  }\n]';
    for (const checkbox of productForm.querySelectorAll('input[name="category_ids"]')) checkbox.checked = false;
    for (const checkbox of productForm.querySelectorAll('input[name="supplier_ids"]')) checkbox.checked = false;
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

  async function refresh(successMessage) {
    try {
      const [products, loadedCategories, loadedBrands, loadedSuppliers] = await Promise.all([
        request("/v1/admin/products"),
        request("/v1/admin/categories"),
        request("/v1/admin/brands"),
        request("/v1/admin/suppliers"),
      ]);
      categories = loadedCategories;
      brands = loadedBrands;
      suppliers = loadedSuppliers;
      renderCategories();
      renderCatalogReferences();
      renderProducts(products);
      if (currentIdentity?.role === "owner") {
        renderStaff(await request("/v1/admin/staff"));
      }
      if (["owner", "admin", "support"].includes(currentIdentity?.role)) {
        renderOrders(await request("/v1/admin/orders"));
      }
      if (["owner", "admin"].includes(currentIdentity?.role)) {
        renderShipping(await request("/v1/admin/shipping"));
        await loadBuildPackageCandidates();
        await loadModuleInventory();
        await loadNewProductsSettings();
      }
      setStatus(successMessage || `${products.length} produit(s), ${categories.length} catégorie(s).`);
    } catch (error) {
      setStatus(error.message, true);
    }
  }

  async function loadModuleInventory(statusMessage) {
    moduleInventory.replaceChildren();
    try {
      const modules = await request("/v1/admin/modules");
      renderModuleInventory(modules);
      moduleInventoryStatus.textContent = statusMessage || (modules.length
        ? `${modules.length} module(s) dans le registre.`
        : "Aucun build de module enregistré.");
      moduleInventoryStatus.dataset.kind = "info";
    } catch (error) {
      moduleInventoryStatus.textContent = error.message;
      moduleInventoryStatus.dataset.kind = "error";
    }
  }

  async function loadBuildPackageCandidates(offset = buildPackageCandidatesOffset, clearDetail = true) {
    const requestID = ++buildPackageCandidatesRequest;
    buildPackageCandidatesOffset = offset;
    buildPackageCandidatesStatus.textContent = "Chargement des packages reçus…";
    buildPackageCandidatesStatus.dataset.kind = "info";
    buildPackageCandidates.replaceChildren();
    if (clearDetail) {
      buildPackageCandidateDetailRequest += 1;
      buildPackageCandidateDetail.replaceChildren();
      buildPackageCandidateDetail.hidden = true;
    }
    try {
      const page = await request(
        `/v1/admin/modules/build-packages/candidates?limit=50&offset=${buildPackageCandidatesOffset}`,
      );
      if (requestID !== buildPackageCandidatesRequest) return;
      renderBuildPackageCandidates(page.items);
      buildPackageCandidatesStatus.textContent = page.total
        ? `${page.total} candidat(s) dans cette boutique.`
        : "Aucun package signé reçu pour cette boutique.";
      buildPackageCandidatesStatus.dataset.kind = "info";
      buildPackageCandidatesPrevious.disabled = buildPackageCandidatesOffset === 0;
      buildPackageCandidatesNext.disabled =
        buildPackageCandidatesOffset + page.items.length >= page.total;
    } catch (error) {
      if (requestID !== buildPackageCandidatesRequest) return;
      buildPackageCandidatesStatus.textContent = error.message;
      buildPackageCandidatesStatus.dataset.kind = "error";
      buildPackageCandidatesPrevious.disabled = true;
      buildPackageCandidatesNext.disabled = true;
    }
  }

  function renderBuildPackageCandidates(candidates) {
    buildPackageCandidates.replaceChildren();
    for (const candidate of candidates) {
      const card = document.createElement("article");
      card.className = "card";
      const title = document.createElement("h4");
      title.textContent = candidate.package_sha256;
      const summary = document.createElement("p");
      summary.className = "muted";
      summary.textContent =
        `${candidate.status === "approved" ? "Approuvé" : "En attente"} · ${formatBytes(candidate.size)}`;
      const details = document.createElement("button");
      details.type = "button";
      details.className = "secondary";
      details.textContent = "Examiner le package";
      details.addEventListener("click", () => loadBuildPackageCandidateDetail(candidate.package_sha256));
      card.append(title, summary, details);
      buildPackageCandidates.append(card);
    }
  }

  async function loadBuildPackageCandidateDetail(digest) {
    const requestID = ++buildPackageCandidateDetailRequest;
    buildPackageCandidateDetail.hidden = false;
    buildPackageCandidateDetail.replaceChildren();
    const loading = document.createElement("p");
    loading.className = "muted";
    loading.textContent = "Revalidation et chargement du candidat…";
    buildPackageCandidateDetail.append(loading);
    try {
      const candidate = await request(
        `/v1/admin/modules/build-packages/candidates/${encodeURIComponent(digest)}`,
      );
      if (requestID !== buildPackageCandidateDetailRequest) return;
      renderBuildPackageCandidateDetail(candidate);
    } catch (error) {
      if (requestID !== buildPackageCandidateDetailRequest) return;
      const message = document.createElement("p");
      message.className = "status";
      message.dataset.kind = "error";
      message.textContent = `Le candidat n’a pas pu être revalidé : ${error.message}`;
      buildPackageCandidateDetail.replaceChildren(message);
    }
  }

  function renderBuildPackageCandidateDetail(candidate) {
    buildPackageCandidateDetail.replaceChildren();
    const title = document.createElement("h4");
    title.textContent = "Détail du package signé";
    const metadata = document.createElement("dl");
    const fields = [
      ["Digest du package", candidate.package_sha256],
      ["Module", candidate.module],
      ["Plateforme", candidate.platform],
      ["SHA-256 Wasm", candidate.wasm_sha256],
      ["Clé signataire", candidate.signer_key_id],
      ["État", candidate.status === "approved" ? "Approuvé" : "En attente d’approbation"],
    ];
    for (const [label, value] of fields) {
      const term = document.createElement("dt");
      term.textContent = label;
      const description = document.createElement("dd");
      description.textContent = value || "Non renseigné";
      metadata.append(term, description);
    }
    buildPackageCandidateDetail.append(title, metadata);
    if (candidate.approval) {
      const approval = document.createElement("p");
      approval.className = "muted";
      approval.textContent =
        `Approuvé par ${candidate.approval.approved_by} (${candidate.approval.approver_role}) le ${new Date(candidate.approval.approved_at).toLocaleString()}.`;
      buildPackageCandidateDetail.append(approval);
      return;
    }
    const note = document.createElement("p");
    note.className = "muted";
    note.textContent = "Une approbation ne publie, n’installe ni ne promeut ce build.";
    const approve = document.createElement("button");
    approve.type = "button";
    approve.textContent = "Approuver ce package";
    approve.addEventListener("click", () => approveBuildPackageCandidate(candidate.package_sha256, approve));
    buildPackageCandidateDetail.append(note, approve);
  }

  async function approveBuildPackageCandidate(digest, button) {
    if (!window.confirm(
      "Approuver ce package après vérification de ses métadonnées ? Cette action ne publie, n’installe ni ne promeut aucun build.",
    )) {
      return;
    }
    button.disabled = true;
    buildPackageCandidatesStatus.textContent = "Enregistrement de l’approbation…";
    buildPackageCandidatesStatus.dataset.kind = "info";
    try {
      await request(`/v1/admin/modules/build-packages/candidates/${encodeURIComponent(digest)}/approval`, {
        method: "POST",
        body: JSON.stringify({ decision: "approve" }),
      });
      buildPackageCandidatesStatus.textContent = "Approbation enregistrée. Aucun build n’a été publié ni activé.";
      buildPackageCandidatesStatus.dataset.kind = "info";
      await loadBuildPackageCandidates(buildPackageCandidatesOffset, false);
      await loadBuildPackageCandidateDetail(digest);
    } catch (error) {
      buildPackageCandidatesStatus.textContent = error.message;
      buildPackageCandidatesStatus.dataset.kind = "error";
      button.disabled = false;
    }
  }

  function formatBytes(size) {
    if (!Number.isFinite(size) || size < 0) return "Taille inconnue";
    if (size < 1024) return `${size} octet(s)`;
    if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} Kio`;
    return `${(size / (1024 * 1024)).toFixed(1)} Mio`;
  }

  function renderModuleInventory(modules) {
    moduleInventory.replaceChildren();
    for (const module of modules) {
      const card = document.createElement("article");
      card.className = "card";
      const title = document.createElement("h3");
      title.textContent = module.name;
      const status = document.createElement("p");
      status.className = "muted";
      const states = {
        installed: "Installé dans cette boutique",
        disabled: "Désinstallé (réactivable)",
        registered: "Build enregistré, non installé",
        error: "Installation en erreur",
      };
      status.textContent = `${states[module.status] || module.status} · ${module.platform || "écosystème inconnu"}`;
      card.append(title, status);
      const metadata = document.createElement("dl");
      const details = [
        ["Version source", module.module_version || "Non renseignée"],
        ["Version plateforme", module.platform_version || "Non renseignée"],
        ["Adaptateur", module.adapter
          ? `${module.adapter}${module.adapter_version ? ` ${module.adapter_version}` : ""}`
          : "Non renseigné"],
        ["ABI", module.abi_version || "Non renseignée"],
        ["Capacités accordées", Array.isArray(module.capabilities) && module.capabilities.length
          ? module.capabilities.join(", ")
          : "Aucune"],
        ["SHA-256 Wasm", module.wasm_sha256 || "Build absent"],
      ];
      if (module.source_reference) details.push(["Référence source", module.source_reference]);
      if (module.source_sha256) details.push(["SHA-256 source", module.source_sha256]);
      if (module.archive_sha256) details.push(["SHA-256 archive", module.archive_sha256]);
      for (const [label, value] of details) {
        const term = document.createElement("dt");
        term.textContent = label;
        const description = document.createElement("dd");
        description.textContent = value;
        metadata.append(term, description);
      }
      card.append(metadata);
      const actions = document.createElement("div");
      actions.className = "actions";
      if (module.can_install && module.status !== "installed") {
        const install = document.createElement("button");
        install.type = "button";
        install.textContent = "Installer dans cette boutique";
        install.addEventListener("click", () => changeModuleInstallation(module.name, true, install));
        actions.append(install);
      }
      if (module.can_uninstall) {
        const uninstall = document.createElement("button");
        uninstall.type = "button";
        uninstall.className = "danger";
        uninstall.textContent = "Désinstaller";
        uninstall.addEventListener("click", () => changeModuleInstallation(module.name, false, uninstall));
        actions.append(uninstall);
      }
      if (actions.childElementCount) card.append(actions);
      const promotableBuilds = (module.builds || []).filter((build) =>
        build.can_promote && !build.installed
      );
      if (module.status === "installed" && promotableBuilds.length) {
        const promotion = document.createElement("div");
        promotion.className = "actions";
        const label = document.createElement("label");
        label.textContent = "Build à activer dans cette boutique";
        const select = document.createElement("select");
        select.setAttribute("aria-label", "Build à activer dans cette boutique");
        for (const build of promotableBuilds) {
          const option = document.createElement("option");
          option.value = build.wasm_sha256;
          const version = build.module_version || "Version inconnue";
          const platform = build.platform || "Écosystème inconnu";
          const markers = [
            build.current ? "build courant" : "",
            build.installed ? "déjà installé" : "",
          ].filter(Boolean);
          option.textContent = `${version} · ${platform} · ${build.wasm_sha256.slice(0, 12)}${markers.length ? ` · ${markers.join(", ")}` : ""}`;
          select.append(option);
        }
        label.append(select);
        const promote = document.createElement("button");
        promote.type = "button";
        promote.textContent = "Activer ce build";
        promote.addEventListener("click", () =>
          promoteModuleBuild(module.name, select.value, promote)
        );
        promotion.append(label, promote);
        card.append(promotion);
      }
      if (module.name === "ps_newproducts" && !module.can_install) {
        const unavailable = document.createElement("p");
        unavailable.className = "muted";
        unavailable.textContent = "Aucun build PrestaShop validé et vérifiable n’est disponible pour l’installation.";
        card.append(unavailable);
      }
      moduleInventory.append(card);
    }
  }

  async function changeModuleInstallation(name, install, button) {
    if (!install && !window.confirm(
      "Désinstaller ce module de la boutique ? Ses réglages et son build seront conservés et le module pourra être réinstallé.",
    )) {
      return;
    }
    button.disabled = true;
    try {
      await request(`/v1/admin/modules/${encodeURIComponent(name)}/installation`, {
        method: install ? "POST" : "DELETE",
      });
      await loadModuleInventory(install
        ? "Module installé dans cette boutique."
        : "Module désinstallé ; le build et les réglages sont conservés.");
      await loadNewProductsSettings();
    } catch (error) {
      moduleInventoryStatus.textContent = error.message;
      moduleInventoryStatus.dataset.kind = "error";
    } finally {
      button.disabled = false;
    }
  }

  async function promoteModuleBuild(name, wasmSHA256, button) {
    if (!window.confirm(
      "Activer ce build remplacera le code exécuté par le module dans cette boutique. Les autres boutiques ne seront pas modifiées.",
    )) {
      return;
    }
    button.disabled = true;
    try {
      await request(`/v1/admin/modules/${encodeURIComponent(name)}/build`, {
        method: "POST",
        body: JSON.stringify({ wasm_sha256: wasmSHA256 }),
      });
      await loadModuleInventory("Build activé dans cette boutique.");
    } catch (error) {
      moduleInventoryStatus.textContent = error.message;
      moduleInventoryStatus.dataset.kind = "error";
    } finally {
      button.disabled = false;
    }
  }

  async function loadNewProductsSettings() {
    newProductsSettingsForm.hidden = true;
    try {
      const settings = await request("/v1/admin/modules/ps_newproducts/config");
      newProductsSettingsForm.elements.days.value = settings.days;
      newProductsSettingsForm.elements.limit.value = settings.limit;
      newProductsSettingsForm.hidden = false;
      moduleSettingsStatus.textContent = "Module installé.";
      moduleSettingsStatus.dataset.kind = "info";
    } catch (error) {
      if (error.status === 404) {
        moduleSettingsStatus.textContent = "Le module ps_newproducts n’est pas installé pour cette boutique.";
        moduleSettingsStatus.dataset.kind = "info";
        return;
      }
      throw error;
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

  for (const tab of adminTabs) {
    tab.addEventListener("click", () => activateAdminTab(tab.dataset.adminTab));
    tab.addEventListener("keydown", (event) => {
      const availableTabs = adminTabs.filter((item) => !item.hidden);
      const index = availableTabs.indexOf(tab);
      let nextIndex;
      if (event.key === "ArrowRight") nextIndex = (index + 1) % availableTabs.length;
      else if (event.key === "ArrowLeft") nextIndex = (index - 1 + availableTabs.length) % availableTabs.length;
      else if (event.key === "Home") nextIndex = 0;
      else if (event.key === "End") nextIndex = availableTabs.length - 1;
      else return;
      event.preventDefault();
      availableTabs[nextIndex].focus();
      activateAdminTab(availableTabs[nextIndex].dataset.adminTab);
    });
  }
  for (const shortcut of document.querySelectorAll("[data-admin-open-tab]")) {
    shortcut.addEventListener("click", () => {
      activateAdminTab(shortcut.dataset.adminOpenTab);
      adminTabs.find((tab) => tab.dataset.adminTab === shortcut.dataset.adminOpenTab && !tab.hidden)?.focus();
    });
  }
  document.querySelector("#refresh-emails").addEventListener("click", () => loadAdminEmails(emailOffset));
  emailsPrevious.addEventListener("click", () => loadAdminEmails(Math.max(0, emailOffset - 50)));
  emailsNext.addEventListener("click", () => loadAdminEmails(emailOffset + 50));
  document.querySelector("#refresh-build-package-candidates").addEventListener("click", () =>
    loadBuildPackageCandidates(buildPackageCandidatesOffset)
  );
  buildPackageCandidatesPrevious.addEventListener("click", () =>
    loadBuildPackageCandidates(Math.max(0, buildPackageCandidatesOffset - 50))
  );
  buildPackageCandidatesNext.addEventListener("click", () =>
    loadBuildPackageCandidates(buildPackageCandidatesOffset + 50)
  );
  window.addEventListener("popstate", activateAdminTabFromHash);

  productForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = new FormData(productForm);
    let translations;
    let media;
    let variants;
    let personalizationOptions;
    try {
      translations = JSON.parse(form.get("translations"));
      media = JSON.parse(form.get("media"));
      variants = JSON.parse(form.get("variants"));
      personalizationOptions = JSON.parse(form.get("personalization_options"));
      if (!Array.isArray(translations) || !Array.isArray(media) || !Array.isArray(variants) ||
          !Array.isArray(personalizationOptions)) throw new Error();
    } catch {
      setStatus("Les traductions, images, variantes et personnalisations doivent être des tableaux JSON valides.", true);
      return;
    }
    const product = {
      slug: form.get("slug"),
      status: form.get("status"),
      brand_id: form.get("brand_id") || "",
      supplier_ids: [...productForm.querySelectorAll('input[name="supplier_ids"]:checked')].map((input) => input.value),
      personalization_options: personalizationOptions,
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

  newProductsSettingsForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const button = newProductsSettingsForm.querySelector('button[type="submit"]');
    button.disabled = true;
    try {
      const settings = await request("/v1/admin/modules/ps_newproducts/config", {
        method: "PUT",
        body: JSON.stringify({
          days: Number(newProductsSettingsForm.elements.days.value),
          limit: Number(newProductsSettingsForm.elements.limit.value),
        }),
      });
      newProductsSettingsForm.elements.days.value = settings.days;
      newProductsSettingsForm.elements.limit.value = settings.limit;
      moduleSettingsStatus.textContent = "Réglages du module enregistrés.";
      moduleSettingsStatus.dataset.kind = "info";
    } catch (error) {
      moduleSettingsStatus.textContent = error.message;
      moduleSettingsStatus.dataset.kind = "error";
    } finally {
      button.disabled = false;
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

  brandForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = new FormData(brandForm);
    const brand = { slug: form.get("slug"), name: form.get("name") };
    const id = form.get("id");
    try {
      await request(id ? `/v1/admin/brands/${encodeURIComponent(id)}` : "/v1/admin/brands", {
        method: id ? "PUT" : "POST",
        body: JSON.stringify(brand),
      });
      resetBrandForm();
      setStatus("Marque enregistrée.");
      await refresh();
    } catch (error) {
      setStatus(error.message, true);
    }
  });

  supplierForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = new FormData(supplierForm);
    const id = form.get("id");
    const supplier = {
      code: form.get("code"),
      name: form.get("name"),
      active: supplierForm.elements.active.checked,
    };
    try {
      await request(id ? `/v1/admin/suppliers/${encodeURIComponent(id)}` : "/v1/admin/suppliers", {
        method: id ? "PUT" : "POST",
        body: JSON.stringify(supplier),
      });
      resetSupplierForm();
      setStatus("Fournisseur enregistré.");
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
  document.querySelector("#cancel-brand-edit").addEventListener("click", resetBrandForm);
  document.querySelector("#cancel-supplier-edit").addEventListener("click", resetSupplierForm);
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
