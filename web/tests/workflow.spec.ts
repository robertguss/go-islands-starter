import {test,expect,type Page} from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
async function login(page:Page,name:string){await page.goto('/preview');await page.getByRole('button',{name:`Continue as ${name}`}).click();await expect(page.getByRole('heading',{name:'Requests',exact:true})).toBeVisible()}

test('request, review, private export, and keyboard dialog',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(m.type()==='error')errors.push(m.text())});
 await login(page,'Alex Morgan');
 await page.getByRole('button',{name:'New request',exact:true}).click();
 const dialog=page.getByRole('dialog');await expect(dialog).toBeVisible();await expect(dialog).toHaveCSS('opacity','1');
 await expect(dialog).toHaveAccessibleName('Create a request');
 await page.keyboard.press('Escape');await expect(dialog).toBeHidden();await expect(page.getByRole('button',{name:'New request',exact:true})).toBeFocused();
 await page.getByRole('button',{name:'New request',exact:true}).click();
 await expect(dialog).toHaveCSS('opacity','1');
 const title=`Browser workflow ${Date.now()}`;
 await page.getByLabel('Title Required').fill(title);await page.getByLabel('Details',{exact:true}).fill('Synthetic browser acceptance record.');
 expect((await new AxeBuilder({page}).analyze()).violations).toEqual([]);
 await page.getByRole('button',{name:'Submit request'}).click();await expect(page.getByRole('status')).toContainText('Request submitted');
 await page.getByLabel('Search requests').fill(title);await page.getByRole('button',{name:'Search',exact:true}).click();await expect(page.locator('#request-list tbody tr')).toHaveCount(1);await expect(page.locator('#request-count')).toHaveText('1');
 await login(page,'Blair Chen');await expect(page.getByText(title,{exact:true})).toHaveCount(0);
 await login(page,'Casey Rivera');const row=page.getByRole('row').filter({hasText:title});await row.getByRole('button',{name:'Approve',exact:true}).click();await expect(page.getByRole('row').filter({hasText:title})).toContainText('Approved');
 await page.getByRole('link',{name:'Exports',exact:true}).click();await page.getByRole('button',{name:'Create CSV export'}).click();
 await expect.poll(async()=>{await page.getByRole('link',{name:'Refresh status'}).click();return page.getByRole('link',{name:'Download CSV'}).count()}).toBeGreaterThan(0);
 const download=page.waitForEvent('download');await page.getByRole('link',{name:'Download CSV'}).first().click();expect((await download).suggestedFilename()).toBe('requests.csv');
 expect(errors).toEqual([]);
});

test('islands survive repeated htmx replacement, sorting, pagination and themes',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 await login(page,'Casey Rivera');await page.getByRole('link',{name:'Overview',exact:true}).click();
 await expect(page.getByRole('heading',{name:'Where things stand'})).toBeVisible();
 for(let i=0;i<8;i++) {await page.getByRole('button',{name:'Refresh overview'}).click();await expect(page.locator('[data-island] table')).toHaveCount(1);await expect(page.getByRole('heading',{name:'Where things stand'})).toBeVisible()}
 await page.getByRole('button',{name:'Request',exact:false}).filter({hasText:'Request'}).click();await expect(page.locator('th').first()).toHaveAttribute('aria-sort','ascending');
 await page.getByRole('button',{name:'Next →'}).click();await expect(page.locator('.pagination')).toContainText('Page 2');
 await page.getByLabel('Filter overview').fill('no-such-record-xyz');await expect(page.getByText('No requests match this filter.')).toBeVisible();await page.getByLabel('Filter overview').fill('');
 expect((await new AxeBuilder({page}).analyze()).violations).toEqual([]);
 await page.getByRole('heading',{name:'Overview',exact:true}).click();await page.evaluate(()=>window.scrollTo(0,0));
 await page.screenshot({path:'test-results/overview-light.png',fullPage:true});
 await page.screenshot({path:'test-results/overview-preview.png'});
 await page.getByRole('button',{name:'Toggle dark theme'}).click();await expect(page.locator('html')).toHaveClass('dark');
 expect((await new AxeBuilder({page}).analyze()).violations).toEqual([]);
 await page.getByRole('heading',{name:'Overview',exact:true}).click();await page.evaluate(()=>window.scrollTo(0,0));
 await page.screenshot({path:'test-results/overview-dark.png',fullPage:true});
 await page.setViewportSize({width:390,height:844});await page.getByRole('heading',{name:'Overview',exact:true}).click();await page.evaluate(()=>window.scrollTo(0,0));
 await page.screenshot({path:'test-results/overview-mobile.png',fullPage:true});
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
 expect(await page.evaluate(()=>Object.keys(localStorage))).toEqual(['theme']);
 expect(errors).toEqual([]);
});

test('access admin sees empty workflow and inaccessible pages reject requests',async({page})=>{
 await login(page,'Dana Patel');await expect(page.getByText('A little room for your next idea.')).toBeVisible();await expect(page.getByRole('button',{name:'New request',exact:true})).toHaveCount(0);
 await page.getByRole('link',{name:'People',exact:true}).click();await expect(page.getByRole('heading',{name:'People',exact:true}).first()).toBeVisible();
 expect((await new AxeBuilder({page}).analyze()).violations).toEqual([]);
 await login(page,'Alex Morgan');const response=await page.goto('/people');expect(response?.status()).toBe(403);await expect(page.getByRole('heading',{name:'Access unavailable'})).toBeVisible();
});
