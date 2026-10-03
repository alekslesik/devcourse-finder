import {createServer} from 'node:http';
import {readFileSync} from 'node:fs';
const {courses}=JSON.parse(readFileSync(new URL('../../data/demo-catalog.json',import.meta.url)));
const result=c=>({course:c,offer:c.offers[0],effective_price:c.offers[0].price,reasons:[],stale:false});
createServer((req,res)=>{res.setHeader('Content-Type','application/json');const url=new URL(req.url,'http://localhost');
 if(url.pathname==='/api/v1/courses'){
 let selected=courses.filter(c=>!url.searchParams.get('language')||c.language===url.searchParams.get('language'));
 if(url.searchParams.has('min'))selected=selected.filter(c=>c.offers[0].price!==null&&c.offers[0].price>=Number(url.searchParams.get('min')));
 if(url.searchParams.has('max'))selected=selected.filter(c=>c.offers[0].price!==null&&c.offers[0].price<=Number(url.searchParams.get('max')));
 const page=Number(url.searchParams.get('page')||1);res.end(JSON.stringify({items:selected.slice((page-1)*12,page*12).map(result),total:selected.length,page,page_size:12}));
 }else if(url.pathname.startsWith('/api/v1/courses/')){const c=courses.find(c=>c.slug===decodeURIComponent(url.pathname.split('/').pop()));if(!c){res.writeHead(404);res.end('{}')}else res.end(JSON.stringify({course:c,offers:c.offers.map(offer=>({...result(c),offer,effective_price:offer.price}))}))}
 else if(url.pathname==='/api/v1/compare'){res.end(JSON.stringify((url.searchParams.get('offer_ids')||'').split(',').map(id=>{const c=courses.find(c=>c.offers.some(o=>o.id===id));return c?{...result(c),offer:c.offers.find(o=>o.id===id)}:{id,unavailable:true}})))}
 else if(url.pathname.startsWith('/out/')){res.writeHead(302,{Location:'https://example.com/course'});res.end()}
 else{res.writeHead(204);res.end()}
}).listen(8091,'127.0.0.1');
