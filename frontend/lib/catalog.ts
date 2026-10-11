import type {components} from './api.generated';
export type Course=components['schemas']['Course'];
export type Offer=components['schemas']['Offer'];
export type Item=components['schemas']['Result'] & {unavailable?:false;id?:never};
export type ComparisonItem=Item | (components['schemas']['Unavailable'] & {course?:never;offer?:never;effective_price?:never});
export type Response=components['schemas']['SearchResponse'];
export function price(i:Item){return i.effective_price===null?'Цена не подтверждена':i.effective_price===0?'Бесплатно':new Intl.NumberFormat('ru-RU',{style:'currency',currency:'RUB',maximumFractionDigits:0}).format(i.effective_price/100)}
export function checked(v:string){const date=new Date(v);if(!Number.isFinite(date.getTime())||date.getUTCFullYear()<=1)return 'Не подтверждено';return date.toLocaleDateString('ru-RU',{timeZone:'Europe/Moscow'})}
export const profiles:Record<string,string>={unknown:'Требования к опыту не подтверждены',none:'Без опыта',basics:'Знаю основы',projects:'Делаю проекты',working:'Работаю разработчиком',switch:'Программирую на другом языке'};

function range(min:number,max:number){const format=new Intl.NumberFormat('ru-RU',{maximumFractionDigits:2});return min===max?format.format(min):format.format(min)+'–'+format.format(max)}
export function duration(o:Offer){
 const d=o.learning_conditions?.duration;
 if(d)return range(d.min,d.max)+' '+({day:'дн.',week:'нед.',month:'мес.',year:'г.'} as Record<string,string>)[d.unit];
 if(o.duration_months && o.duration_months>0)return o.duration_months+' мес.';
 if(o.weeks && o.weeks>0)return o.weeks+' нед.';
 return null;
}

export function workload(o:Offer){const w=o.learning_conditions?.weekly_workload;return w?range(w.min,w.max)+' ч/нед.':o.hours!=null?o.hours+' ч/нед.':null}
export function supportValue(o:Offer,kind:'review'|'mentor'):boolean|null{const v=o.learning_conditions?.[kind];if(v)return v.available;return o.support_known===false?null:o[kind]}
export function supportText(o:Offer,kind:'review'|'mentor'){const v=supportValue(o,kind);return v===null?'Уточните у школы':v?'Есть':'Нет'}
export function supportLabel(o:Offer){if(supportValue(o,'mentor')===true)return 'Персональный наставник';if(supportValue(o,'review')===true)return 'Проверка заданий';if(o.learning_conditions?.community?.available)return 'Учебное сообщество';return supportValue(o,'mentor')===false&&supportValue(o,'review')===false?'Самостоятельно':null}
