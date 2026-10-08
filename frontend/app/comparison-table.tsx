import ExternalMark from './external-mark';
import type {ComparisonItem} from '../lib/catalog';
import {price, checked} from '../lib/catalog';
const rows = ['Тариф','Полная стоимость','Длительность','Нагрузка','Проверка кода','Индивидуальные занятия','Набор','Дата проверки','Действия'];
export default function ComparisonTable({items,ids,onRemove}:{items:ComparisonItem[];ids:string[];onRemove?:(id:string)=>void}) {
  return <div className="tableWrap" role="region" aria-label="Сравнение тарифов, горизонтальная прокрутка" tabIndex={0}><table>
    <thead><tr><th scope="col">Условия</th>{items.map((i,n)=><th scope="col" key={ids[n]}>{i.unavailable?(i.reason==='closed'?'Набор закрыт — предложение недоступно':'Предложение недоступно'):<a href={'/courses/'+encodeURIComponent(i.course.slug)}>{i.course.title}</a>}</th>)}</tr></thead>
    <tbody>{rows.map((row,r)=><tr key={row}><th scope="row">{row}</th>{items.map((i,n)=><td key={ids[n]}>{r===8?<>
      {onRemove?<button className="textButton" onClick={()=>onRemove(ids[n])} aria-label={'Удалить тариф '+(n+1)}>Удалить</button>:<a href={'/compare?offers='+ids.filter((_,j)=>j!==n).map(encodeURIComponent).join(',')}>Удалить</a>}
      {!i.unavailable&&<p><a className="secondary linkButton" href={'/out/'+encodeURIComponent(i.offer.id)} target="_blank" rel="noopener noreferrer">Проверить условия <ExternalMark/></a></p>}
    </>:i.unavailable?'Недоступно':r===0?i.offer.name:r===1?price(i):r===2?(i.offer.weeks===null?'Не указана':i.offer.weeks+' недель'):r===3?(i.offer.hours===null?'Не указана':i.offer.hours+' ч/нед.'):r===4?(i.offer.support_known===false?'Не подтверждено':i.offer.review?'Есть':'Нет'):r===5?(i.offer.support_known===false?'Не подтверждено':i.offer.mentor?'Есть':'Нет'):r===6?({open:'Открыт',continuous:'Постоянный',closed:'Закрыт',unknown:'Неизвестен'} as Record<string,string>)[i.offer.enrollment]:checked(i.course.checked_at)}</td>)}</tr>)}</tbody>
  </table></div>;
}
