const api = process.env.API_URL || 'http://api:8080';
export default { output: 'standalone', experimental: {useTypeScriptCli: false}, poweredByHeader: false, async rewrites() { return [
{source:'/api/:path*',destination:api+'/api/:path*'},
{source:'/out/:path*',destination:api+'/out/:path*'}
]; } };