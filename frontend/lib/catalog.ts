import type {components} from './api.generated';
export type Course=components['schemas']['Course'];
export type Offer=components['schemas']['Offer'];
export type Item=components['schemas']['Result'] & {unavailable?:false;id?:never};
export type ComparisonItem=Item | (components['schemas']['Unavailable'] & {course?:never;offer?:never;effective_price?:never});
export type Response=components['schemas']['SearchResponse'];
export function price(i:Item){return i.effective_price===null?'Цена не подтверждена':i.effective_price===0?'Бесплатно':new Intl.NumberFormat('ru-RU',{style:'currency',currency:'RUB',maximumFractionDigits:0}).format(i.effective_price/100)}
export function checked(v:string){const date=new Date(v);if(!Number.isFinite(date.getTime())||date.getUTCFullYear()<=1)return 'Не подтверждено';return date.toLocaleDateString('ru-RU',{timeZone:'Europe/Moscow'})}
export const profiles:Record<string,string>={unknown:'Требования к опыту не подтверждены',none:'Без опыта',basics:'Знаю основы',projects:'Делаю проекты',working:'Работаю разработчиком',switch:'Программирую на другом языке'};
