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
 const sitemap=await (await fetch(origin+'/sitemap.xml')).text();assert.ok(sitemap.includes('https://courses.example/courses/'+course.slug));assert.ok(!sitemap.includes('/archived'));
 outage=true;const failed=await fetch(origin+'/courses/'+course.slug);assert.equal(failed.status,500);
 }finally{child.kill();await new Promise(resolve=>child.once('close',resolve));await new Promise(resolve=>api.close(resolve))}
});
