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
  let token = null;
  let categories = [];
  let editingCategory = null;
  let currentIdentity = null;

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
    currentIdentity = identity;
    loginPanel.hidden = true;
    dashboard.hidden = false;
    logoutButton.hidden = false;
    document.querySelector("#staff-panel").hidden = identity.role !== "owner";
    document.querySelector("#orders-panel").hidden =
      !["owner", "admin", "support"].includes(identity.role);
    document.querySelector("#identity").textContent =
      `${identity.name || identity.email} · ${identity.email} · rôle ${identity.role}`;
  }

  function clearSession() {
    token = null;
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
    heading.textContent = `Commande ${order.number} · ${order.status}`;
    const customer = document.createElement("p");
    customer.textContent = `${order.name} · ${order.email}`;
    const total = document.createElement("p");
    total.textContent = `Total : ${order.grand_total} ${order.currency}`;
    const items = document.createElement("ul");
    for (const item of order.items) {
      const row = document.createElement("li");
      row.textContent = `${item.quantity} × ${item.name} (${item.sku}) · ${item.line_total} ${order.currency}`;
      items.append(row);
    }
    const address = document.createElement("p");
    address.textContent = `${order.shipping_address.line1 || ""} · ${order.shipping_address.postal_code || ""} ${order.shipping_address.city || ""} · ${order.shipping_address.country || ""}`;
    orderDetail.append(heading, customer, total, items, address);
  }

  function renderOrders(orders) {
    orderList.replaceChildren();
    orderDetail.hidden = true;
    for (const order of orders) {
      const card = document.createElement("article");
      card.className = "card";
      const title = document.createElement("h3");
      title.textContent = `${order.number} · ${order.status}`;
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
    productForm.elements.variants.value = '[\n  {\n    "sku": "ARTICLE-001",\n    "options": {},\n    "prices": [{ "currency": "EUR", "amount": "12.0000" }],\n    "inventory": { "quantity": 10, "track": true }\n  }\n]';
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
