import { Page } from 'playwright-core';

export interface SomMark {
  markId: number;
  tagName: string;
  role: string;
  text: string;
  bbox: { x: number; y: number; width: number; height: number };
}

export class SomEngine {
  private markCache: Map<number, SomMark> = new Map();

  public async annotate(page: Page, filter = 'clickable'): Promise<{ imageBase64: string; marks: SomMark[] }> {
    const marks: SomMark[] = await page.evaluate((mode) => {
      const old = document.getElementById('__virium_som_overlay__');
      if (old) old.remove();

      const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
      svg.id = '__virium_som_overlay__';
      svg.style.position = 'fixed';
      svg.style.top = '0';
      svg.style.left = '0';
      svg.style.width = '100vw';
      svg.style.height = '100vh';
      svg.style.pointerEvents = 'none';
      svg.style.zIndex = '2147483647';

      const selector = 'button, a[href], input, select, textarea, [role="button"], [role="link"], [role="checkbox"], [role="menuitem"], [role="tab"], [onclick]';
      const elements = Array.from(document.querySelectorAll(selector));

      const markList: any[] = [];
      let currentId = 1;

      elements.forEach((el) => {
        const rect = el.getBoundingClientRect();
        if (rect.width < 5 || rect.height < 5) return;
        if (rect.bottom < 0 || rect.top > window.innerHeight) return;
        if (rect.right < 0 || rect.left > window.innerWidth) return;

        const style = window.getComputedStyle(el);
        if (style.display === 'none' || style.visibility === 'hidden' || style.opacity === '0') return;

        const hit = document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2);
        if (hit && !el.contains(hit) && !hit.contains(el)) return;

        // Bounding Box
        const rectElem = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
        rectElem.setAttribute('x', String(rect.left));
        rectElem.setAttribute('y', String(rect.top));
        rectElem.setAttribute('width', String(rect.width));
        rectElem.setAttribute('height', String(rect.height));
        rectElem.setAttribute('fill', 'rgba(255, 0, 85, 0.08)');
        rectElem.setAttribute('stroke', '#FF0055');
        rectElem.setAttribute('stroke-width', '2');
        rectElem.setAttribute('rx', '3');
        svg.appendChild(rectElem);

        // Badge
        const badgeW = 22;
        const badgeH = 16;
        const badgeBg = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
        badgeBg.setAttribute('x', String(Math.max(0, rect.left)));
        badgeBg.setAttribute('y', String(Math.max(0, rect.top - badgeH)));
        badgeBg.setAttribute('width', String(badgeW));
        badgeBg.setAttribute('height', String(badgeH));
        badgeBg.setAttribute('fill', '#FF0055');
        badgeBg.setAttribute('rx', '2');
        svg.appendChild(badgeBg);

        // Text
        const textElem = document.createElementNS('http://www.w3.org/2000/svg', 'text');
        textElem.setAttribute('x', String(Math.max(0, rect.left) + badgeW / 2));
        textElem.setAttribute('y', String(Math.max(0, rect.top - badgeH) + 12));
        textElem.setAttribute('fill', '#FFFFFF');
        textElem.setAttribute('font-size', '11px');
        textElem.setAttribute('font-weight', 'bold');
        textElem.setAttribute('text-anchor', 'middle');
        textElem.setAttribute('font-family', 'sans-serif');
        textElem.textContent = String(currentId);
        svg.appendChild(textElem);

        el.setAttribute('data-som-id', String(currentId));
        markList.push({
          markId: currentId,
          tagName: el.tagName.toLowerCase(),
          role: el.getAttribute('role') || el.tagName.toLowerCase(),
          text: (el.textContent || el.getAttribute('aria-label') || '').trim().slice(0, 50),
          bbox: { x: rect.left, y: rect.top, width: rect.width, height: rect.height }
        });
        currentId++;
      });

      document.body.appendChild(svg);
      return markList;
    }, filter);

    this.markCache.clear();
    marks.forEach((m) => this.markCache.set(m.markId, m));

    const buffer = await page.screenshot({ type: 'png', fullPage: false });

    await page.evaluate(() => {
      const overlay = document.getElementById('__virium_som_overlay__');
      if (overlay) overlay.remove();
    });

    return {
      imageBase64: buffer.toString('base64'),
      marks,
    };
  }

  public async interactWithMark(page: Page, markId: number, action: string, text?: string): Promise<void> {
    const mark = this.markCache.get(markId);
    if (!mark) throw new Error(`Mark #${markId} not found in active snapshot`);

    const locator = page.locator(`[data-som-id="${markId}"]`);
    if ((await locator.count()) > 0) {
      if (action === 'click') await locator.first().click();
      else if (action === 'type' && text) await locator.first().fill(text);
      else if (action === 'hover') await locator.first().hover();
    } else {
      // Bounding box fallback
      const cx = mark.bbox.x + mark.bbox.width / 2;
      const cy = mark.bbox.y + mark.bbox.height / 2;
      if (action === 'click') await page.mouse.click(cx, cy);
      else if (action === 'type' && text) {
        await page.mouse.click(cx, cy);
        await page.keyboard.type(text);
      }
    }
  }
}
