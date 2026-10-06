// Interaksi kecil sisi klien. Semua handler memakai event delegation pada document
// sehingga tetap bekerja setelah konten diganti oleh htmx.
(function () {
  "use strict";

  const fmt = (n) => Math.round(n).toLocaleString("id-ID");
  // "1.500.000" -> 1500000 ; qty "1,5" -> 1.5
  const parseMoney = (s) => parseInt(String(s || "").replace(/[^\d-]/g, ""), 10) || 0;
  const parseQty = (s) => parseFloat(String(s || "").replace(",", ".")) || 0;

  // ---- Form transaksi: subtotal & total ----
  function recalcOrder() {
    const rows = document.querySelectorAll("#items .item-row");
    if (!rows.length) return;
    let total = 0;
    rows.forEach((tr) => {
      const sub = parseQty(tr.querySelector(".qty").value) * parseMoney(tr.querySelector(".price").value);
      tr.querySelector(".subtotal").textContent = fmt(sub);
      total += Math.round(sub);
    });
    const el = document.getElementById("grand-total");
    if (el) el.textContent = fmt(total);
    return total;
  }

  // ---- Form invoice: total transaksi terpilih ----
  function recalcInvoice() {
    const table = document.querySelector("[data-invoice-table]");
    if (!table) return;
    let total = 0;
    table.querySelectorAll('input[name="order_id"]:checked').forEach((cb) => (total += parseInt(cb.dataset.amount, 10) || 0));
    const el = table.querySelector("[data-selected-total]");
    if (el) el.textContent = fmt(total);
  }

  const iso = (d) => new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 10);

  document.addEventListener("change", (e) => {
    const t = e.target;
    // Pilih produk master -> isi nama, satuan, harga.
    if (t.matches(".product-select")) {
      const opt = t.selectedOptions[0];
      const tr = t.closest("tr");
      if (opt && opt.value !== "0") {
        tr.querySelector('[name="product_name"]').value = opt.dataset.name;
        tr.querySelector('[name="unit"]').value = opt.dataset.unit;
        tr.querySelector('[name="price"]').value = fmt(parseInt(opt.dataset.price, 10));
      }
      recalcOrder();
    }
    if (t.matches("[data-check-all]")) {
      t.closest("table").querySelectorAll('input[name="order_id"]').forEach((cb) => (cb.checked = t.checked));
    }
    if (t.closest("[data-invoice-table]")) recalcInvoice();
  });

  document.addEventListener("input", (e) => {
    if (e.target.closest("#items")) recalcOrder();
  });

  // Format ribuan saat input nominal kehilangan fokus.
  document.addEventListener("focusout", (e) => {
    if (e.target.matches("input.money") && e.target.value.trim() !== "") {
      e.target.value = fmt(parseMoney(e.target.value));
    }
  });

  document.addEventListener("click", (e) => {
    const t = e.target;
    if (t.closest(".remove-row")) {
      const rows = document.querySelectorAll("#items .item-row");
      if (rows.length > 1) t.closest("tr").remove();
      else t.closest("tr").querySelectorAll("input").forEach((i) => (i.value = i.classList.contains("qty") ? "1" : ""));
      recalcOrder();
    }
    // Uang muka = total (bayar lunas di muka).
    if (t.matches("[data-pay-full]")) {
      const total = recalcOrder() || 0;
      document.querySelector('[name="dp_amount"]').value = fmt(total);
    }
    // Pintasan periode laporan.
    if (t.matches("[data-range]")) {
      const now = new Date();
      let from, to = now;
      switch (t.dataset.range) {
        case "this-month": from = new Date(now.getFullYear(), now.getMonth(), 1); break;
        case "last-month":
          from = new Date(now.getFullYear(), now.getMonth() - 1, 1);
          to = new Date(now.getFullYear(), now.getMonth(), 0);
          break;
        case "this-year": from = new Date(now.getFullYear(), 0, 1); break;
        default: from = new Date(2000, 0, 1);
      }
      const form = t.closest("form");
      form.querySelector('[name="from"]').value = iso(from);
      form.querySelector('[name="to"]').value = iso(to);
      form.requestSubmit();
    }
  });

  // Hitung ulang setelah halaman dimuat / di-swap htmx.
  const init = () => { recalcOrder(); recalcInvoice(); };
  document.addEventListener("DOMContentLoaded", init);
  document.addEventListener("htmx:afterSettle", init);
})();
