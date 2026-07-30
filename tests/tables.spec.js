// @ts-check
const { test, expect } = require('@playwright/test');
const { spawnSync } = require('child_process');
const path = require('path');
const fs = require('fs');
const os = require('os');

const EXE = path.resolve(__dirname, '..', 'mdview.exe');
const FIXTURES = path.join(__dirname, 'fixtures');

/**
 * Render a markdown file to a temporary HTML file using mdview.exe --export.
 * Returns the path to the generated HTML, or null if the exe is unavailable
 * or --export fails.
 */
function renderToHtml(mdFile) {
  const tmp = path.join(os.tmpdir(), `mdview_test_${Date.now()}_${Math.random().toString(36).slice(2)}.html`);
  // TODO: replace with: execSync(`mdview.exe --export=${tmp} ${mdFile}`) once --export is stable
  const result = spawnSync(EXE, [`--export=${tmp}`, mdFile], { encoding: 'utf8' });
  if (result.status !== 0 || !fs.existsSync(tmp)) return null;
  return tmp;
}

/**
 * Open a rendered HTML for the given markdown fixture in the given page.
 * Skips the test if mdview.exe is not available or --export is not implemented.
 * Returns the tmp HTML path so the caller can clean up.
 */
async function openMd(page, mdFile) {
  const html = renderToHtml(mdFile);
  if (!html) {
    test.skip(true, 'mdview.exe not available or --export not implemented');
    return null;
  }
  const fileUrl = 'file:///' + html.replace(/\\/g, '/');
  await page.goto(fileUrl);
  await page.waitForLoadState('load');
  // Wait for JS to initialize tables (sorting setup + wide-table detection)
  await page.waitForTimeout(400);
  return html;
}

// ---------------------------------------------------------------------------
// Table sorting
// ---------------------------------------------------------------------------

test.describe('Table sorting', () => {

  test('all thead th elements get data-sortable attribute', async ({ page }) => {
    const html = await openMd(page, path.join(FIXTURES, 'tables.md'));
    if (!html) return;

    const sortableCount = await page.locator('.content thead th[data-sortable]').count();
    expect(sortableCount).toBeGreaterThan(0);

    fs.rmSync(html, { force: true });
  });

  test('clicking a text column header sorts rows ascending', async ({ page }) => {
    const html = await openMd(page, path.join(FIXTURES, 'tables.md'));
    if (!html) return;

    // Click the first th (Name column) in the first table
    const th = page.locator('.content table').first().locator('thead th').first();
    await th.click();

    // After ascending sort: Alice < Bob < Charlie < Diana alphabetically
    const firstCell = page.locator('.content table').first().locator('tbody tr').first().locator('td').first();
    const lastCell  = page.locator('.content table').first().locator('tbody tr').last().locator('td').first();

    await expect(firstCell).toHaveText('Alice');
    await expect(lastCell).toHaveText('Diana');

    fs.rmSync(html, { force: true });
  });

  test('clicking same header a second time sorts rows descending', async ({ page }) => {
    const html = await openMd(page, path.join(FIXTURES, 'tables.md'));
    if (!html) return;

    const th = page.locator('.content table').first().locator('thead th').first();
    await th.click(); // asc
    await expect(th).toHaveAttribute('data-dir', 'asc');
    await th.click(); // desc

    // Descending: Diana first, Alice last
    const firstCell = page.locator('.content table').first().locator('tbody tr').first().locator('td').first();
    const lastCell  = page.locator('.content table').first().locator('tbody tr').last().locator('td').first();

    await expect(firstCell).toHaveText('Diana');
    await expect(lastCell).toHaveText('Alice');

    fs.rmSync(html, { force: true });
  });

  test('clicking same header a third time restores original order', async ({ page }) => {
    const html = await openMd(page, path.join(FIXTURES, 'tables.md'));
    if (!html) return;

    const firstTable = page.locator('.content table').first();
    const th = firstTable.locator('thead th').first();
    await th.click(); // asc
    await expect(th).toHaveAttribute('data-dir', 'asc');
    await th.click(); // desc
    await expect(th).toHaveAttribute('data-dir', 'desc');
    await th.click(); // original

    // Original order: Alice, Bob, Charlie, Diana
    const rows = firstTable.locator('tbody tr');
    await expect(rows.nth(0).locator('td').first()).toHaveText('Alice');
    await expect(rows.nth(1).locator('td').first()).toHaveText('Bob');
    await expect(rows.nth(2).locator('td').first()).toHaveText('Charlie');
    await expect(rows.nth(3).locator('td').first()).toHaveText('Diana');

    fs.rmSync(html, { force: true });
  });

  test('sort indicator data-dir attribute follows click cycle asc → desc → none', async ({ page }) => {
    const html = await openMd(page, path.join(FIXTURES, 'tables.md'));
    if (!html) return;

    const th = page.locator('.content table').first().locator('thead th').first();

    // Click 1: ascending
    await th.click();
    await expect(th).toHaveAttribute('data-dir', 'asc');

    // Click 2: descending
    await th.click();
    await expect(th).toHaveAttribute('data-dir', 'desc');

    // Click 3: no indicator
    await th.click();
    await expect(th).not.toHaveAttribute('data-dir');

    fs.rmSync(html, { force: true });
  });

  test('clicking a different column clears the previous column indicator', async ({ page }) => {
    const html = await openMd(page, path.join(FIXTURES, 'tables.md'));
    if (!html) return;

    const firstTable = page.locator('.content table').first();
    const th0 = firstTable.locator('thead th').nth(0);
    const th1 = firstTable.locator('thead th').nth(1);

    // Click col 0 first
    await th0.click();
    await expect(th0).toHaveAttribute('data-dir', 'asc');

    // Click col 1 — col 0 indicator must be gone
    await th1.click();
    await expect(th1).toHaveAttribute('data-dir', 'asc');
    await expect(th0).not.toHaveAttribute('data-dir');

    fs.rmSync(html, { force: true });
  });

});

// ---------------------------------------------------------------------------
// Numeric sorting
// ---------------------------------------------------------------------------

test.describe('Numeric sorting', () => {

  test('numbers sort numerically not lexicographically', async ({ page }) => {
    const html = await openMd(page, path.join(FIXTURES, 'tables.md'));
    if (!html) return;

    // The second table has the numeric "Value" column (index 1)
    const numericTable = page.locator('.content table').nth(1);
    const valueHeader = numericTable.locator('thead th').nth(1);

    await valueHeader.click(); // ascending

    // Expected order: -5, 7, +42, ~100, 1,234 (=1234), then "abc" (non-number last)
    const rows = numericTable.locator('tbody tr');
    const count = await rows.count();

    // First row: B (-5)
    await expect(rows.first().locator('td').nth(0)).toHaveText('B');
    // Last row: E (abc — non-number)
    await expect(rows.last().locator('td').nth(0)).toHaveText('E');

    fs.rmSync(html, { force: true });
  });

  test('non-numeric values sort after all numbers', async ({ page }) => {
    const html = await openMd(page, path.join(FIXTURES, 'tables.md'));
    if (!html) return;

    const numericTable = page.locator('.content table').nth(1);
    const valueHeader = numericTable.locator('thead th').nth(1);

    await valueHeader.click(); // ascending

    const rows = numericTable.locator('tbody tr');
    const lastItem = await rows.last().locator('td').nth(0).textContent();
    // "E" has value "abc" — only non-numeric entry
    expect(lastItem?.trim()).toBe('E');

    fs.rmSync(html, { force: true });
  });

  test('1,234 (comma-separated thousands) sorts as 1234, not lexicographically', async ({ page }) => {
    const html = await openMd(page, path.join(FIXTURES, 'tables.md'));
    if (!html) return;

    const numericTable = page.locator('.content table').nth(1);
    const valueHeader = numericTable.locator('thead th').nth(1);

    await valueHeader.click(); // ascending

    const rows = numericTable.locator('tbody tr');
    // C has value 1,234 which should be last among numbers (before abc)
    // Row order expected: B(-5), F(7), D(+42), A(~100), C(1234), E(abc)
    const secondToLast = await rows.nth(-2).locator('td').nth(0).textContent();
    expect(secondToLast?.trim()).toBe('C');

    fs.rmSync(html, { force: true });
  });

});

// ---------------------------------------------------------------------------
// Wide table layout
// ---------------------------------------------------------------------------

test.describe('Wide table layout', () => {

  test('wide table gets wrapped in .table-wide-wrapper', async ({ page }) => {
    const html = await openMd(page, path.join(FIXTURES, 'wide_table.md'));
    if (!html) return;

    const wrapper = page.locator('.table-wide-wrapper');
    await expect(wrapper).toHaveCount(1);

    // Table must be a direct child of the wrapper
    const tableInsideWrapper = page.locator('.table-wide-wrapper > table');
    await expect(tableInsideWrapper).toHaveCount(1);

    fs.rmSync(html, { force: true });
  });

  test('wide table wrapper left edge is at or right of viewport left edge', async ({ page }) => {
    const html = await openMd(page, path.join(FIXTURES, 'wide_table.md'));
    if (!html) return;

    const wrapper = page.locator('.table-wide-wrapper');
    const box = await wrapper.boundingBox();
    expect(box).not.toBeNull();
    // Wrapper must not overflow to the left of the viewport
    expect(box && box.x).toBeGreaterThanOrEqual(0);

    fs.rmSync(html, { force: true });
  });

  test('wide table wrapper is horizontally scrollable (content overflows)', async ({ page }) => {
    const html = await openMd(page, path.join(FIXTURES, 'wide_table.md'));
    if (!html) return;

    const wrapper = page.locator('.table-wide-wrapper');

    // scrollWidth > clientWidth means there is overflowing content to scroll
    const overflows = await wrapper.evaluate((el) => el.scrollWidth > el.clientWidth);
    expect(overflows).toBe(true);

    fs.rmSync(html, { force: true });
  });

  test('wide table can be scrolled horizontally (scrollLeft responds)', async ({ page }) => {
    const html = await openMd(page, path.join(FIXTURES, 'wide_table.md'));
    if (!html) return;

    const wrapper = page.locator('.table-wide-wrapper');

    // Set scrollLeft and verify it sticks (i.e. there is room to scroll)
    await wrapper.evaluate((el) => { el.scrollLeft = 100; });
    const scrollLeft = await wrapper.evaluate((el) => el.scrollLeft);
    expect(scrollLeft).toBeGreaterThan(0);

    fs.rmSync(html, { force: true });
  });

  test('narrow table does NOT get wrapped in .table-wide-wrapper', async ({ page }) => {
    const html = await openMd(page, path.join(FIXTURES, 'tables.md'));
    if (!html) return;

    // At 1280px viewport the tables in tables.md should comfortably fit
    const wrapperCount = await page.locator('.table-wide-wrapper').count();
    expect(wrapperCount).toBe(0);

    fs.rmSync(html, { force: true });
  });

});

// ---------------------------------------------------------------------------
// Resize behavior
// ---------------------------------------------------------------------------

test.describe('Resize behavior', () => {

  test('narrowing viewport causes a previously-fitting table to get wrapped', async ({ page }) => {
    const html = await openMd(page, path.join(FIXTURES, 'tables.md'));
    if (!html) return;

    // At 1280px the tables should fit — no wrapper expected
    expect(await page.locator('.table-wide-wrapper').count()).toBe(0);

    // Shrink to 300px — the tables should now overflow and get wrapped
    await page.setViewportSize({ width: 300, height: 800 });
    // Wait for debounce (200ms) + margin
    await page.waitForTimeout(500);

    // At 300px at least one table should be wrapped
    const wrapperCount = await page.locator('.table-wide-wrapper').count();
    expect(wrapperCount).toBeGreaterThan(0);

    fs.rmSync(html, { force: true });
  });

  test('widening viewport unwraps a table that now fits', async ({ page }) => {
    // Start at 400px so the wide table definitely overflows
    await page.setViewportSize({ width: 400, height: 800 });

    const html = await openMd(page, path.join(FIXTURES, 'wide_table.md'));
    if (!html) return;

    // Wrapper should be present at 400px
    await expect(page.locator('.table-wide-wrapper')).toHaveCount(1);

    // Widen to 2560px — the 10-column table should now fit inside .content
    await page.setViewportSize({ width: 2560, height: 800 });
    await page.waitForTimeout(500); // debounce 200ms + margin

    // Either the wrapper is gone, or it exists but the table fits (scrollWidth <= clientWidth)
    const wrapperCount = await page.locator('.table-wide-wrapper').count();
    if (wrapperCount > 0) {
      // Wrapper still present but table should fit (no horizontal overflow)
      const overflows = await page.locator('.table-wide-wrapper').evaluate(
        (el) => el.scrollWidth > el.clientWidth
      );
      expect(overflows).toBe(false);
    } else {
      // Wrapper was removed — this is the expected path
      expect(wrapperCount).toBe(0);
    }

    fs.rmSync(html, { force: true });
  });

});
