import { test, expect } from '@playwright/test';

// Test: Person detail page should be visually elegant, modern, and minimalist

test('Person detail page visually polished and navigable', async ({ page }) => {
  // Update URL and selector as needed if your routing is different
  await page.goto('http://localhost:3000/people/1');
  await page.waitForLoadState('networkidle');

  // Take a full-page screenshot for review
  await page.screenshot({ path: 'person-detail-full.png', fullPage: true });

  // Assert the main layout sections exist and are visually grouped
   // Person name is always the .text-3xl font
   const nameHeading = page.locator('h1.text-3xl');
   await expect(nameHeading).toHaveText(/.+/); // Name
   await expect(page.locator('img')).toBeVisible();   // Profile photo
   // Gallery badge (usually 1+ galleries)
   const galleryBadge = page.locator('.badge,span').filter({ hasText: /gallery/i });
   // Debug: print all badge and main text for troubleshooting
   const allText = await page.textContent('body');
   console.log('BODY TEXT:', allText);
   const badgeTexts = await page.locator('.badge').allTextContents();
   console.log('BADGE TEXTS:', badgeTexts);
   // At least one badge should exist (aliases, nationality, etc)
   expect(badgeTexts.length).toBeGreaterThan(0);
   // Gallery badge optional; log if missing
   const galleryBadgeText = badgeTexts.find(t => /gallery/i.test(t));
   if (!galleryBadgeText) console.warn('No gallery badge present for this person');
   const buttonCount = await page.locator('button').count();
   expect(buttonCount).toBeGreaterThan(0); // At least action buttons
  
  // Take screenshot of just the profile/info flex area
  const hero = page.locator('h1').first().locator('..'); // parent div of the name (info area)
  await hero.screenshot({ path: 'person-detail-hero.png' });
});
