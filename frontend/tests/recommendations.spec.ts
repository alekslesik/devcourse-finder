import {test,expect} from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import {readFileSync} from 'node:fs';
const base=JSON.parse(readFileSync('../data/demo-catalog.json','utf8')).courses[0];
test('AI recommendations explain matches and can be excluded without hiding unknowns',async({page})=>{
 await page.setViewportSize({width:360,height:800});
 const quote='Программа включает основы Go, HTTP, API и практический проект для освоения нового языка.';
 const tags=(values:string[])=>({values,quote,evidence:{source:base.source,locator:'section:program',digest:'a'.repeat(64),checked_at:new Date().toISOString()}});
 await page.route('**/api/v1/courses*',route=>{
  const p=new URL(route.request().url()).searchParams,include=p.get('include_recommendations')==='true';
  const confirmed={course:{...base,title:'Подтверждённый курс'},offer:base.offers[0],effective_price:0,reasons:['Подходит вашему опыту'],stale:false,profile_match:'confirmed'};
  const recommended={...confirmed,course:{...base,id:'estimate',title:'Практика Go по программе',recommendations:{audience:tags(['basics']),goals:tags(['job']),directions:tags(['backend'])}},offer:{...base.offers[0],id:'estimated-offer'},profile_match:'recommended',recommendation_fields:['experience','goal','direction'],reasons:['Оценка ИИ: подготовка к первой работе']};
  const unknown={...confirmed,course:{...base,id:'unknown',title:'Курс с неполными данными'},offer:{...base.offers[0],id:'unknown-offer'},profile_match:'unverified',unknown_profile_fields:['experience'],reasons:[]};
  const counts=(key:string)=>({eligible:3,unknown:2,selected:key,counts:{[key]:1},recommended_counts:{[key]:include?1:0}});
  return route.fulfill({json:{items:include?[confirmed,recommended,unknown]:[confirmed,unknown],total:include?3:2,confirmed_total:1,recommended_total:include?1:0,unverified_total:1,page:1,page_size:12,diagnostics:{experience:counts('basics'),goal:counts('job'),direction:counts('backend')}}});
 });
 await page.goto('/courses?language=go&experience=basics&goal=job&direction=backend');
 await expect(page.locator('select[name=experience] option[value=basics]')).toHaveText('Знаю основы · 1 + 1 ИИ');
 const group=page.locator('.recommendedResults');await expect(group.getByRole('heading',{name:'Могут подойти по оценке ИИ'})).toBeVisible();
 await expect(group.locator('.reasons')).not.toContainText('✓');
 await group.getByText('Почему ИИ предлагает этот курс',{exact:true}).click();
 await expect(group.locator('blockquote').first()).toHaveText(quote);
 await expect(group.getByRole('link',{name:'Программа курса'}).first()).toHaveAttribute('href',base.source);
 await expect(page.locator('.unverifiedResults')).toContainText('Не хватает данных: подходящий опыт.');
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
 const axe=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();expect(axe.violations.filter(v=>v.impact==='serious'||v.impact==='critical')).toEqual([]);
 await page.getByRole('checkbox',{name:'Учитывать оценки ИИ',exact:true}).uncheck();
 await page.getByRole('button',{name:'Найти обучение'}).click();
 await expect(page).toHaveURL(/include_recommendations=false/);await expect(page.locator('.recommendedResults')).toHaveCount(0);await expect(page.locator('.unverifiedResults .card')).toHaveCount(1);
 await page.reload();await expect(page.getByRole('checkbox',{name:'Учитывать оценки ИИ',exact:true})).not.toBeChecked();await expect(page.locator('.recommendedResults')).toHaveCount(0);
 await page.goBack();await expect(page.getByRole('checkbox',{name:'Учитывать оценки ИИ',exact:true})).toBeChecked();await expect(page.locator('.recommendedResults')).toBeVisible();
});


test('course detail keeps AI interpretations separate from unconfirmed requirements',async({page})=>{
 await page.goto('/courses/passport-advisory');
 const overview=page.locator('.recommendationOverview');
 await expect(overview.getByRole('heading',{name:'Оценка ИИ по программе'})).toBeVisible();
 await expect(overview).toContainText('Начинающим с нуля');await expect(overview).toContainText('Данные и машинное обучение');
 await expect(page.getByText('Требования к опыту не подтверждены',{exact:true})).toBeVisible();
 await overview.getByText('На чём основана оценка ИИ',{exact:true}).click();await expect(overview.locator('blockquote').first()).toContainText('практический проект');
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
});
