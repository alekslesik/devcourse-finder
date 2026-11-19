import {readFileSync} from 'node:fs';
const version = readFileSync(new URL('../VERSION', import.meta.url), 'utf8').trim();
if (!/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(version)) throw new Error('Invalid root VERSION');
const api = process.env.API_URL || 'http://api:8080';
export default { output: 'standalone', env: {NEXT_PUBLIC_APP_VERSION: version}, experimental: {useTypeScriptCli: false}, poweredByHeader: false, async headers(){return [{source:'/account/:path*',headers:[
 {key:'Cache-Control',value:'private, no-store, max-age=0'},
 {key:'Referrer-Policy',value:'no-referrer'},
 {key:'X-Robots-Tag',value:'noindex, nofollow, noarchive'},
 {key:'Content-Security-Policy',value:"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; font-src 'self'; frame-ancestors 'none'; base-uri 'self'; object-src 'none'; form-action 'self'"}
 ]}]}, async rewrites() { return {fallback:[
{source:'/api/:path*',destination:api+'/api/:path*'},
{source:'/out/:path*',destination:api+'/out/:path*'}
]}; } };
