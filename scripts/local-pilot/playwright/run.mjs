import assert from "node:assert/strict";
import { readFileSync, statSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { chromium } from "playwright";

const scriptDirectory = dirname(fileURLToPath(import.meta.url));
const repositoryRoot = resolve(scriptDirectory, "../../..");
const args = process.argv.slice(2);

if (args.includes("--help")) {
  console.log("Usage: node run.mjs [--base-url URL] [--credentials FILE] [--no-login] [--headed]");
  process.exit(0);
}

function optionValue(name) {
  const inline = args.find((argument) => argument.startsWith(`${name}=`));
  if (inline) return inline.slice(name.length + 1);
  const index = args.indexOf(name);
  return index >= 0 ? args[index + 1] : undefined;
}

const baseUrl = (optionValue("--base-url") ?? process.env.CMS_BASE_URL ?? "http://127.0.0.1:8081").replace(/\/+$/, "");
const skipLogin = args.includes("--no-login");
const headed = args.includes("--headed");

function loadCredentials() {
  const email = process.env.CMS_TEST_EMAIL;
  const password = process.env.CMS_TEST_PASSWORD;
  if (email || password) {
    if (!email || !password) {
      throw new Error("Set both CMS_TEST_EMAIL and CMS_TEST_PASSWORD");
    }
    return { email, password };
  }

  const credentialsPath = process.env.CMS_TEST_CREDENTIALS ?? resolve(repositoryRoot, "CMS-TEST-CREDENTIALS.txt");
  let contents;
  try {
    const mode = statSync(credentialsPath).mode;
    if (process.platform !== "win32" && (mode & 0o077) !== 0) {
      throw new Error("credentials file permissions must be private (0600)");
    }
    contents = readFileSync(credentialsPath, "utf8");
  } catch (error) {
    if (error.code === "ENOENT") return null;
    throw error;
  }

  const credentials = {};
  for (const line of contents.split(/\r?\n/)) {
    const separator = line.indexOf(":");
    if (separator < 0) continue;
    const key = line.slice(0, separator).trim().toLowerCase();
    const value = line.slice(separator + 1).trim();
    if (key.includes("email")) credentials.email = value;
    if (key.includes("password")) credentials.password = value;
  }
  if (!credentials.email || !credentials.password) {
    throw new Error("credentials file must contain Admin email and Admin password fields");
  }
  return credentials;
}

const credentials = skipLogin ? null : loadCredentials();
if (credentials) {
  const hostname = new URL(baseUrl).hostname;
  if (!["localhost", "127.0.0.1", "[::1]"].includes(hostname)) {
    throw new Error("credential-based login is restricted to loopback URLs");
  }
}

const browser = await chromium.launch({ headless: !headed });
try {
  const page = await browser.newPage();

  const healthResponse = await page.goto(`${baseUrl}/health`);
  assert.equal(healthResponse?.status(), 200, "CMS health endpoint should return HTTP 200");
  console.log("PASS /health");

  const adminResponse = await page.goto(`${baseUrl}/admin/`);
  assert.equal(adminResponse?.status(), 200, "admin page should return HTTP 200");
  assert.match(await page.title(), /Administration/, "admin page title should be present");

  if (credentials) {
    await page.getByRole("textbox", { name: "E-mail" }).first().fill(credentials.email);
    await page.getByRole("textbox", { name: "Mot de passe" }).first().fill(credentials.password);
    await page.getByRole("button", { name: "Se connecter" }).click();
    await page.getByRole("button", { name: "Se déconnecter" }).waitFor({ timeout: 10000 });
    await page.getByRole("heading", { name: "Préparer un essai boutique" }).waitFor();
    await page.getByRole("tab", { name: "Catalogue" }).click();
    assert.equal(new URL(page.url()).hash, "#catalogue", "catalog tab should be directly addressable");
    await page.getByRole("tabpanel", { name: "Catalogue" }).waitFor();
    await page.getByRole("tab", { name: "Boutique" }).click();
    await page.getByRole("heading", { name: "Préparer un essai boutique" }).waitFor();
    console.log("PASS admin login, dashboard, and section navigation");
  } else {
    await page.getByRole("heading", { name: "Connexion staff" }).waitFor();
    console.log("PASS admin login page (authentication skipped)");
  }

  const storefrontResponse = await page.goto(`${baseUrl}/storefront/`);
  assert.equal(storefrontResponse?.status(), 200, "storefront should return HTTP 200");
  const products = page.getByRole("list", { name: "Produits" }).getByRole("listitem");
  await products.first().waitFor();
  const productCount = await products.count();
  assert.ok(productCount > 0, "storefront should render at least one product");
  console.log(`PASS storefront (${productCount} products)`);
} finally {
  await browser.close();
}