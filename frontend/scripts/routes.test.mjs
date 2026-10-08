import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {spawn} from 'node:child_process';
import {readFileSync} from 'node:fs';
const {courses}=JSON.parse(readFileSync(new URL('../../data/demo-catalog.json',import.meta.url)));
const course=courses[0];
const result={course,offer:course.offers[0],effective_price:course.offers[0].price,reasons:[],stale:false};
test('production routes: server HTML, unpublished 404, comparison URL, sitemap and outage',async()=>{
 let outage=false;
 const api=createServer((req,res)=>{
  res.setHeader('Content-Type','application/json');
  const url=new URL(req.url,'http://localhost');
  if(outage){res.writeHead(503);res.end('{}');return}
  if(url.pathname==='/api/v1/courses')res.end(JSON.stringify({items:[result],total:1,page:1,page_size:48}));
  else if(url.pathname==='/api/v1/compare')res.end(JSON.stringify(url.searchParams.get('offer_ids').split(',').map(id=>id===result.offer.id?result:{id,unavailable:true})));
  else if(url.pathname==='/api/v1/courses/'+course.slug)res.end(JSON.stringify({course,offers:[result]}));
  else if(url.pathname==='/api/v1/courses/archived')res.end(JSON.stringify({course:{...course,status:'archived'},offers:[result]}));
  else{res.writeHead(404);res.end('{}')}
 });
 await new Promise(resolve=>api.listen(0,'127.0.0.1',resolve));
 const apiPort=api.address().port;
 const web=createServer();await new Promise(resolve=>web.listen(0,'127.0.0.1',resolve));const webPort=web.address().port;await new Promise(resolve=>web.close(resolve));
 const child=spawn(process.env.TEST_NODE||process.execPath,['node_modules/next/dist/bin/next','start','-p',String(webPort),'-H','127.0.0.1'],{env:{...process.env,API_URL:'http://127.0.0.1:'+apiPort,SITE_URL:'https://courses.example'},stdio:['ignore','pipe','pipe']});
 let output='';child.stdout.on('data',data=>output+=data);child.stderr.on('data',data=>output+=data);
 try{
 const origin='http://127.0.0.1:'+webPort;
 let ready=false;for(let attempt=0;attempt<100;attempt++){try{if((await fetch(origin+'/about')).ok){ready=true;break}}catch{}await new Promise(resolve=>setTimeout(resolve,100))}assert.ok(ready,output);
 const detail=await fetch(origin+'/courses/'+course.slug);assert.equal(detail.status,200);const html=await detail.text();assert.ok(html.includes(course.title));assert.match(html,/canonical/);assert.ok(html.includes('/out/'+result.offer.id));
 for(const slug of ['missing','archived'])assert.equal((await fetch(origin+'/courses/'+slug)).status,404);
 const compare=await fetch(origin+'/compare?offers='+result.offer.id+',removed');const comparison=await compare.text();assert.equal(compare.status,200);assert.match(comparison,/Предложение недоступно/);assert.match(comparison,/noindex/);
 // Crawler responses must contain complete metadata in the initial HTML head.
 const expected=[['/','DevCourseFinder — найдите свой путь в разработку'],['/courses?language=go','Каталог курсов для разработчиков — DevCourseFinder'],['/about','О сервисе — DevCourseFinder'],['/compare?offers='+result.offer.id,'Сравнение тарифов — DevCourseFinder'],['/courses/'+course.slug,course.title+' — DevCourseFinder']];
 const meta=(head,name)=>{
  const tag=head.match(new RegExp('<meta (?:property|name)="'+name+'" content="([^\"]*)"'));
  assert.ok(tag,`Missing ${name} in the HTML head`);
  return tag[1].replaceAll('&amp;','&').replaceAll('&quot;','"').replaceAll('&#x27;',"'").replaceAll('&lt;','<').replaceAll('&gt;','>');
 };
 for(const agent of ['TelegramBot (like TwitterBot)','WhatsApp/2.24.0','facebookexternalhit/1.1']){
  for(const [path,title] of expected){
   const response=await fetch(origin+path,{headers:{'User-Agent':agent}});assert.equal(response.status,200);
   const body=await response.text(),head=body.slice(0,body.indexOf('</head>'));
   assert.equal(meta(head,'og:title'),title);assert.equal(meta(head,'twitter:title'),title);
   assert.ok(meta(head,'og:description'));assert.equal(meta(head,'og:description'),meta(head,'twitter:description'));
   assert.equal(new URL(meta(head,'og:url')).href,new URL(path.split('?')[0],'https://courses.example').href);
   assert.equal(meta(head,'og:site_name'),'DevCourseFinder');assert.equal(meta(head,'og:locale'),'ru_RU');
   assert.equal(meta(head,'twitter:card'),'summary_large_image');
   assert.equal(meta(head,'og:image'),'https://courses.example/social-preview-v2.png');
   assert.equal(meta(head,'twitter:image'),meta(head,'og:image'));
   assert.equal(meta(head,'og:image:width'),'1200');assert.equal(meta(head,'og:image:height'),'630');
   if(path.startsWith('/courses/'))assert.equal(meta(head,'og:description'),course.summary);
  }
 }
 const image=await fetch(origin+'/social-preview-v2.png',{headers:{'User-Agent':'TelegramBot'}});
 assert.equal(image.status,200);assert.match(image.headers.get('content-type'),/^image\/png/);
 const png=Buffer.from(await image.arrayBuffer());assert.ok(png.length>10000&&png.length<5*1024*1024);
 assert.equal(png.subarray(0,8).toString('hex'),'89504e470d0a1a0a');
 assert.equal(png.readUInt32BE(16),1200);assert.equal(png.readUInt32BE(20),630);
 // Browser and iOS icons must be served as real image assets, not fallback HTML.
 for(const [path,size] of [['/favicon-32.png',32],['/apple-touch-icon.png',180]]){
  const icon=await fetch(origin+path);assert.equal(icon.status,200);assert.match(icon.headers.get('content-type'),/^image\/png/);
  const bytes=Buffer.from(await icon.arrayBuffer());assert.equal(bytes.subarray(0,8).toString('hex'),'89504e470d0a1a0a');
  assert.equal(bytes.readUInt32BE(16),size);assert.equal(bytes.readUInt32BE(20),size);
 }
 const vector=await fetch(origin+'/favicon.svg');assert.equal(vector.status,200);assert.match(vector.headers.get('content-type'),/^image\/svg\+xml/);
 const sitemap=await (await fetch(origin+'/sitemap.xml')).text();assert.ok(sitemap.includes('https://courses.example/courses/'+course.slug));assert.ok(!sitemap.includes('/archived'));
 outage=true;const failed=await fetch(origin+'/courses/'+course.slug);assert.equal(failed.status,500);
 }finally{child.kill();await new Promise(resolve=>child.once('close',resolve));await new Promise(resolve=>api.close(resolve))}
});
