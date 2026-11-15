import * as THREE from "three";
import { ClassicLayout } from "../../web/layout_classic-layout";
import { ConstraintLayout, Operator, Strength } from "../../web/layout_constraint-layout";
import { Scene3D } from "../../web/render3d_scene";
import { Spring } from "../../web/spring";
import { SearchBar, filterByQuery } from "../../web/searchbar";
import { ProgressBar } from "../../web/progressbar";
import { ThemeController } from "../../web/theme";
import { drawBarChart, drawLineChart, type BarDatum } from "../../web/charts";
import { downloadCSV, downloadXLSX, parseCSV } from "../../web/export";
import { parseXLSX } from "../../web/export_xlsx-read";
import { DataTable } from "../../web/datatable";
import { NotificationCenter } from "../../web/notifications";
import { UploadZone } from "../../web/upload";
import { ChatStore, ChatView, actionButton, copyText, copyToClipboard } from "../../web/chat";
import { generateKeyPair, exportPublicKey, importPublicKey, deriveSharedKey, encryptMessage, decryptMessage } from "../../web/chat_crypto";

interface Item {
  Slug: string;
  Title: string;
  PriceCents: number;
  Currency: string;
  HTML: string;
}

// --- shell: one classic header/sidebar/main/footer layout, the shape most pages actually use.
// ConstraintLayout stays available as an opt-in module (demoed in its own bounded box below), it's
// just no longer what the whole page is built on.
const header = document.createElement("header");
const sidebar = document.createElement("aside");
const mainEl = document.createElement("main");
const footer = document.createElement("footer");

const classicLayout = new ClassicLayout(
  document.getElementById("app")!,
  { header, sidebar, main: mainEl, footer },
  { sidebarWidth: "260px" },
);
// Without this, the grid is only ever laid out once (at construction time) and never reacts to
// the window actually being resized afterwards - exactly the "looks fine on load, breaks the
// moment you resize the window" bug.
classicLayout.observeResize();

// --- notifications: one stack for the whole page (upload results, item-load failures, ...).
const notifications = new NotificationCenter(document.body);

// --- header: title, theme toggle, search.
header.innerHTML = `<h1>fastwebapp</h1>`;
const themeBtn = document.createElement("button");
themeBtn.className = "fwa-theme-toggle";
header.append(themeBtn);
const searchHost = document.createElement("div");
header.append(searchHost);

const theme = new ThemeController(["dark", "light"], "dark");
const renderThemeBtn = () => (themeBtn.textContent = theme.theme === "dark" ? "🌙 dark" : "☀️ light");
renderThemeBtn();
themeBtn.addEventListener("click", () => {
  theme.cycle();
  renderThemeBtn();
});

const searchBar = new SearchBar(searchHost, { placeholder: "Search items…" });

// --- sidebar: nav between the demo sections.
sidebar.innerHTML = `
  <h2>fastwebapp</h2>
  <p>Go backend · TS frontend · one classic layout, everything else is an opt-in module</p>
  <nav>
    <a href="#items">Items</a>
    <a href="#charts">Charts</a>
    <a href="#upload">Upload</a>
    <a href="#chat">Chat</a>
    <a href="#constraint-demo">Constraint layout (opt-in)</a>
  </nav>
`;

footer.textContent = "fastwebapp - self-hosted Rails-like framework";

// --- main: hero, items table, charts, upload, chat, constraint-layout demo, in that order.
const hero = document.createElement("div");
hero.id = "hero";
mainEl.append(hero);

const itemsSection = document.createElement("section");
itemsSection.id = "items";
itemsSection.innerHTML = `<h2>Items</h2>`;
const exportButtons = document.createElement("div");
exportButtons.className = "fwa-export-buttons";
const csvBtn = document.createElement("button");
csvBtn.textContent = "Export CSV";
const xlsxBtn = document.createElement("button");
xlsxBtn.textContent = "Export XLSX";
exportButtons.append(csvBtn, xlsxBtn);
itemsSection.append(exportButtons);
const itemsTableHost = document.createElement("div");
itemsSection.append(itemsTableHost);
mainEl.append(itemsSection);

const chartsSection = document.createElement("section");
chartsSection.id = "charts";
chartsSection.innerHTML = `<h2>Charts</h2>`;
const barCanvas = document.createElement("canvas");
barCanvas.width = 480;
barCanvas.height = 180;
const lineCanvas = document.createElement("canvas");
lineCanvas.width = 480;
lineCanvas.height = 180;
chartsSection.append(barCanvas, lineCanvas);
mainEl.append(chartsSection);

const uploadSection = document.createElement("section");
uploadSection.id = "upload";
uploadSection.innerHTML = `<h2>Upload</h2><p>Every upload also gets a .torrent and a magnet link, for grabbing it over BitTorrent instead of plain HTTP.</p>`;
const progressHost = document.createElement("div");
const progress = new ProgressBar(progressHost);
const uploadZoneHost = document.createElement("div");
const downloadsListHost = document.createElement("div");
downloadsListHost.className = "fwa-downloads-list";
const previewHost = document.createElement("div");
previewHost.className = "fwa-downloads-preview";
uploadSection.append(uploadZoneHost, progressHost, downloadsListHost, previewHost);
mainEl.append(uploadSection);

interface DownloadListing {
  id: string;
  filename: string;
  sizeBytes: number;
  contentType: string;
  infoHash: string;
  uploadedAt: string;
  fileURL: string;
  torrentURL: string;
  magnet: string;
}

// CSV/XLSX previewing isn't tied to any particular upload - it's just "fetch this file's bytes
// and render them as a table" - so it reuses the same parseCSV/parseXLSX this page already has
// for the Items export buttons, applied to whatever file the user just uploaded.
async function previewSpreadsheet(entry: DownloadListing): Promise<void> {
  previewHost.innerHTML = "";
  try {
    let rows: Record<string, string>[];
    if (entry.filename.toLowerCase().endsWith(".csv")) {
      rows = parseCSV(await fetch(entry.fileURL).then((r) => r.text()));
    } else {
      const buffer = await fetch(entry.fileURL).then((r) => r.arrayBuffer());
      rows = await parseXLSX(new Uint8Array(buffer));
    }
    if (rows.length === 0) {
      previewHost.textContent = `${entry.filename}: no data rows to preview.`;
      return;
    }
    new DataTable<Record<string, unknown>>(previewHost, {
      pageSize: 10,
      columns: Object.keys(rows[0]!).map((key) => ({ key, label: key, sortable: true })),
    }).setRows(rows as Record<string, unknown>[]);
  } catch (err) {
    notifications.notify(`preview failed: ${err}`, "error");
  }
}

function isPreviewable(filename: string): boolean {
  const lower = filename.toLowerCase();
  return lower.endsWith(".csv") || lower.endsWith(".xlsx");
}

function renderDownloadEntry(entry: DownloadListing): HTMLElement {
  const row = document.createElement("div");
  row.className = "fwa-download-entry";
  row.innerHTML = `
    <strong>${entry.filename}</strong> (${(entry.sizeBytes / 1024).toFixed(1)} KB)
    <a href="${entry.fileURL}" download>file</a>
    <a href="${entry.torrentURL}">torrent</a>
    <a href="${entry.magnet}">magnet</a>
  `;
  if (isPreviewable(entry.filename)) {
    const previewBtn = document.createElement("button");
    previewBtn.textContent = "Preview";
    previewBtn.addEventListener("click", () => previewSpreadsheet(entry));
    row.append(previewBtn);
  }
  return row;
}

async function loadDownloads(): Promise<void> {
  let entries: DownloadListing[] = [];
  try {
    entries = await fetch("/downloads").then((r) => r.json());
  } catch (err) {
    notifications.notify(`could not load downloads: ${err}`, "error");
    return;
  }
  downloadsListHost.innerHTML = "";
  for (const entry of entries) downloadsListHost.append(renderDownloadEntry(entry));
}
loadDownloads();

new UploadZone(uploadZoneHost, {
  url: "/downloads",
  maxSizeBytes: 10 * 1024 * 1024,
  onProgress: (percent) => progress.set(percent),
  onSuccess: (response) => {
    progress.set(100);
    const entry: DownloadListing = JSON.parse(response);
    notifications.notify(`uploaded: ${entry.filename}`, "success");
    loadDownloads();
  },
  onError: (message) => {
    progress.set(0);
    notifications.notify(message, "error");
  },
});

const chatSection = document.createElement("section");
chatSection.id = "chat";
chatSection.innerHTML = `<h2>Chat (end-to-end encrypted demo)</h2>`;
const chatHost = document.createElement("div");
chatSection.append(chatHost);
mainEl.append(chatSection);

// This demo is a chatbot-style use case, so it opts into Copy/Fork/Regenerate itself via
// renderActions - the chat module's core tree logic doesn't know or care that these exist. A plain
// messenger app would pass a different renderActions (or none) using the same hook.
const chatStore = new ChatStore();
const chatView = new ChatView(chatHost, chatStore, {
  renderActions: (message, store) => {
    const buttons: HTMLElement[] = [
      actionButton("Copy", "fwa-chat-action", () => copyToClipboard(copyText(message))),
      actionButton("Fork", "fwa-chat-action", () => {
        const text = prompt("Fork with new text:");
        if (text == null) return;
        const forked = store.fork(message.id, text);
        chatView.render(forked.id);
      }),
    ];
    if (message.role === "assistant") {
      buttons.push(
        actionButton("Regenerate", "fwa-chat-action", () => {
          const regenerated = store.regenerate(message.id, "(regenerated reply)");
          chatView.render(regenerated.id);
        }),
      );
    }
    const { index, count } = store.siblingIndex(message.id);
    if (count > 1) {
      const switcher = document.createElement("span");
      switcher.className = "fwa-chat-branch-switcher";
      switcher.textContent = `${index + 1}/${count}`;
      buttons.push(switcher);
    }
    return buttons;
  },
});

// Demo: two independent keypairs derive the same shared key and round-trip an encrypted
// message, proving the module end-to-end rather than just appending plaintext bubbles.
async function runChatCryptoDemo(): Promise<void> {
  const alice = await generateKeyPair();
  const bob = await generateKeyPair();
  const aliceSharedKey = await deriveSharedKey(alice.privateKey, await importPublicKey(await exportPublicKey(bob.publicKey)));
  const bobSharedKey = await deriveSharedKey(bob.privateKey, await importPublicKey(await exportPublicKey(alice.publicKey)));

  const userMsg = chatStore.add(null, "user", "hey, is this actually encrypted?");
  const encrypted = await encryptMessage(aliceSharedKey, "yes - this round-tripped through AES-GCM with an ECDH-derived key");
  const decrypted = await decryptMessage(bobSharedKey, encrypted);
  const assistantMsg = chatStore.add(userMsg.id, "assistant", decrypted);

  chatView.render(assistantMsg.id);
  notifications.notify("chat demo: message encrypted with ECDH+AES-GCM and decrypted by the other party's key", "info");
}
runChatCryptoDemo();

// --- constraint layout: opt-in module, demoed in its own bounded box rather than driving the
// whole page (that's classic-layout's job now). Same 0.3x sidebar/main relationship as before.
const constraintSection = document.createElement("section");
constraintSection.id = "constraint-demo";
constraintSection.innerHTML = `<h2>Constraint layout (opt-in module)</h2><p>Sidebar is always exactly 0.3x of main's width, inside this box only.</p>`;
const constraintBox = document.createElement("div");
constraintBox.className = "fwa-constraint-demo-box";
const constraintSidebar = document.createElement("div");
constraintSidebar.className = "fwa-constraint-demo-sidebar";
constraintSidebar.textContent = "sidebar";
const constraintMain = document.createElement("div");
constraintMain.className = "fwa-constraint-demo-main";
constraintMain.textContent = "main";
constraintBox.append(constraintSidebar, constraintMain);
constraintSection.append(constraintBox);
mainEl.append(constraintSection);

const cl = new ConstraintLayout(constraintBox);
const cSidebar = cl.addBox(constraintSidebar);
const cMain = cl.addBox(constraintMain);

cl.constrain(cSidebar.left, Operator.Eq, 0);
cl.constrain(cSidebar.top, Operator.Eq, 0);
cl.constrain(cMain.top, Operator.Eq, 0);
cl.constrain(cSidebar.height, Operator.Eq, cl.container.height);
cl.constrain(cMain.height, Operator.Eq, cl.container.height);
cl.constrain(cMain.left, Operator.Eq, cSidebar.left.plus(cSidebar.width));
cl.constrain(cSidebar.width, Operator.Eq, cMain.width.multiply(0.3), Strength.strong);
cl.constrain(cSidebar.width.plus(cMain.width), Operator.Eq, cl.container.width);
cl.observeResize();

// --- 3D hero: a rotating icosahedron, mounted into #hero. This is the opt-in module, not the
// framework's foundation - the rest of the page is ordinary DOM.
const scene3d = new Scene3D(hero, { cameraDistance: 3.2 });
const mesh = new THREE.Mesh(
  new THREE.IcosahedronGeometry(1, 0),
  new THREE.MeshStandardMaterial({ color: 0x7fd1a8, flatShading: true }),
);
scene3d.scene.add(mesh);
scene3d.onFrame((dt) => {
  mesh.rotation.x += dt * 0.4;
  mesh.rotation.y += dt * 0.6;
});

// --- items from the Go backend: a searchable, sortable, paginated table (datatable + searchbar
// + pagination modules), each card still lifted on hover by a spring, plus CSV/XLSX export.
async function loadItems(): Promise<void> {
  let items: Item[] = [];
  try {
    items = await fetch("/items").then((r) => r.json());
  } catch (err) {
    notifications.notify(`Could not load items - is the Go backend running? (${err})`, "error");
    return;
  }

  const table = new DataTable<Item & Record<string, unknown>>(itemsTableHost, {
    pageSize: 5,
    columns: [
      { key: "Title", label: "Title", sortable: true },
      {
        key: "PriceCents",
        label: "Price",
        sortable: true,
        render: (row) => `${(row.PriceCents / 100).toFixed(2)} ${row.Currency.toUpperCase()}`,
      },
      { key: "Currency", label: "Currency", sortable: true },
    ],
  });

  const applyFilter = (query: string) => {
    const filtered = filterByQuery(items, query, (item) => item.Title);
    table.setRows(filtered as (Item & Record<string, unknown>)[]);
  };
  applyFilter("");
  searchBar.onQuery(applyFilter);

  csvBtn.addEventListener("click", () => downloadCSV("items.csv", items as unknown as Record<string, unknown>[]));
  xlsxBtn.addEventListener("click", () => downloadXLSX("items.xlsx", items as unknown as Record<string, unknown>[]));

  // Charts: price per item, bar for the absolute values and a line for the same series - the
  // demo's own sample data, not a second fetch.
  const bars: BarDatum[] = items.map((item) => ({ label: item.Title, value: item.PriceCents / 100 }));
  drawBarChart(barCanvas, bars);
  drawLineChart(lineCanvas, bars.map((b) => b.value));

  const itemsListHost = document.createElement("div");
  itemsListHost.className = "fwa-items-hover-list";
  itemsSection.append(itemsListHost);
  for (const item of items) {
    const card = document.createElement("div");
    card.className = "card";
    card.innerHTML = `<h3>${item.Title}</h3><div class="price">${(item.PriceCents / 100).toFixed(2)} ${item.Currency.toUpperCase()}</div>`;

    const lift = new Spring(0, { stiffness: 300, damping: 20 });
    lift.subscribe((y) => (card.style.transform = `translateY(${y}px)`));
    card.addEventListener("mouseenter", () => lift.set(-6));
    card.addEventListener("mouseleave", () => lift.set(0));

    itemsListHost.append(card);
  }
}

loadItems();
