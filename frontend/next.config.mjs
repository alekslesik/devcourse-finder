import {readFileSync} from 'node:fs';
const version = readFileSync(new URL('../VERSION', import.meta.url), 'utf8').trim();
if (!/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(version)) throw new Error('Invalid root VERSION');
const api = process.env.API_URL || 'http://api:8080';
export default { output: 'standalone', env: {NEXT_PUBLIC_APP_VERSION: version}, experimental: {useTypeScriptCli: false}, poweredByHeader: false, async rewrites() { return [
{source:'/api/:path*',destination:api+'/api/:path*'},
{source:'/out/:path*',destination:api+'/out/:path*'}
]; } };
