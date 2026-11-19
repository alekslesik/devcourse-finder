// Server-only transport: this module imports node:crypto and must never enter a client bundle.
import {createHash,createHmac} from 'node:crypto';
export function actionSignature(key,{method,path,time,nonce,ip,origin='',csrf='',cookie=''},body){
 const digest=createHash('sha256').update(body).digest('hex');
 return createHmac('sha256',key).update(['devcourse-finder/action-proxy/v1',method,path,time,nonce,ip,origin,csrf,cookie,digest].join('\n')).digest('hex');
}
