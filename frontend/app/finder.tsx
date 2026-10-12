'use client';
import {useEffect,useState,useRef} from 'react';
import type {Response,ComparisonItem} from '../lib/catalog';
import Modal from './modal';
import ComparisonTable from './comparison-table';
import CourseCard from './course-card';
import ResultState from './result-state';
import {recordEvent as event} from '../lib/analytics';
const unavailableMessage='Каталог временно недоступен. Попробуйте ещё раз.';
class SearchError extends Error {}
const languages:Record<string,string>={python:'Python',javascript:'JavaScript',java:'Java',go:'Go'};
const experience:Record<string,string>={none:'Начинаю с нуля',basics:'Знаю основы',projects:'Пишу свои проекты',working:'Работаю разработчиком',switch:'Перехожу с другого языка'};
const goals:Record<string,string>={try:'Попробовать программирование',job:'Подготовиться к первой работе',switch:'Сменить язык / направление',deepen:'Углубить знания'};
const directions:Record<string,string>={basics:'Основы',backend:'Backend',frontend:'Frontend',fullstack:'Fullstack',automation:'Автоматизация'};
function Select({label,name,values,defaultValue,counts}:{label:string;name:string;values:Record<string,string>;defaultValue?:string;counts?:Record<string,number>}){return <label>{label}<select name={name} defaultValue={defaultValue||''}><option value="">Не важно</option>{Object.entries(values).map(([v,t])=><option value={v} key={v}>{t}{counts ? ` · ${counts[v]??0}` : ''}</option>)}</select></label>}
const searchParameterNames=new Set(['language','direction','experience','goal','budget','support','schedule','sort','min','max','hours','include_free','include_closed','include_unknown_profiles','page','page_size']);
const urlParameterNames=new Set([...searchParameterNames,'compare']);
function canonicalParams(source:URLSearchParams){const result=new URLSearchParams();source.forEach((value,key)=>{if(!urlParameterNames.has(key)||!value||key==='sort'&&value==='recent'||key==='page'&&value==='1'||key==='page_size'&&value==='12'||value==='false')return;result.set(key,value)});return result}
function searchParams(source:URLSearchParams){const result=new URLSearchParams();source.forEach((value,key)=>{if(searchParameterNames.has(key))result.set(key,value)});return result}
function countLabel(n:number){const tail=n%100, digit=n%10;return `${n} ${tail>=11&&tail<=14?'программ':digit===1?'программа':digit>=2&&digit<=4?'программы':'программ'}`}
const filterLabels:Record<string,string>={language:'Язык',direction:'Направление',experience:'Опыт',goal:'Цель',budget:'Стоимость',support:'Поддержка',schedule:'Расписание',min:'От',max:'До',hours:'Часов в неделю',include_free:'Бесплатные включены',include_closed:'Закрытый набор включён',include_unknown_profiles:'Неполные данные включены'};
function filterValue(key:string,value:string){
 if(key==='min'||key==='max')return new Intl.NumberFormat('ru-RU').format(Number(value)/100)+' ₽';
 const values:Record<string,Record<string,string>>={language:languages,experience,goal:goals,direction:directions,budget:{free:'Бесплатно',paid:'Платные'},support:{self:'Самостоятельно',review:'Проверка заданий',mentor:'Наставник'},schedule:{flexible:'В своём темпе',scheduled:'По расписанию'}};
 return values[key]?.[value]||(value==='true'?'Да':value);
}
export default function Finder({resultsPath='/courses'}:{resultsPath?:string}){
 const [formRevision,setFormRevision]=useState(0);
 const [params,setParams]=useState<URLSearchParams|null>(null),[data,setData]=useState<Response|null>(null),[error,setError]=useState(''),[loading,setLoading]=useState(true),[retry,setRetry]=useState(0);
 const [announcement,setAnnouncement]=useState('');
 const [selected,setSelected]=useState<string[]>([]),[compare,setCompare]=useState<ComparisonItem[]|null>(null),[compareOpen,setCompareOpen]=useState(false),[modalError,setModalError]=useState(''),[notice,setNotice]=useState('');
 useEffect(()=>{const read=()=>{const raw=new URLSearchParams(location.search),p=canonicalParams(raw),query=p.toString();if(raw.toString()!==query)history.replaceState(null,'',location.pathname+(query?'?'+query:''));setParams(p);setFormRevision(v=>v+1);if(p.has('compare'))setSelected([...new Set((p.get('compare')||'').split(',').filter(Boolean))].slice(0,3));setCompareOpen(p.has('compare'))};read();window.addEventListener('popstate',read);return()=>window.removeEventListener('popstate',read)},[]);
 useEffect(()=>{
  if(!params)return;
  const ctrl=new AbortController(),requestParams=searchParams(params);requestParams.set('diagnostics','true');const query=requestParams.toString();
  setLoading(true);setError('');setAnnouncement('');
  fetch('/api/v1/courses'+(query?'?'+query:''),{signal:ctrl.signal}).then(async r=>{
   if(!r.ok)throw new SearchError(r.status===400?'Проверьте параметры поиска или сбросьте фильтры.':'Каталог временно недоступен. Попробуйте ещё раз.');
   const data=await r.json();
   if(!Array.isArray(data.items)||typeof data.total!=='number')throw new SearchError(unavailableMessage);
   return data;
  }).then((d:Response)=>{
   // Transport may already have completed when cleanup aborts this search.
   // Ignore queued results and analytics as well as late failures.
   if(ctrl.signal.aborted)return;
   setData(d);setAnnouncement(focusResults.current?'':d.total?countLabel(d.total)+' найдено.':'Подходящих программ нет.');event(d.total?'search':'empty',{language:params.get('language')||'',goal:params.get('goal')||'',total:d.total});
  }).catch(e=>{if(!ctrl.signal.aborted)setError(e instanceof SearchError?e.message:unavailableMessage)}).finally(()=>{if(!ctrl.signal.aborted)setLoading(false)});
  return()=>ctrl.abort();
 },[params,retry]);
 useEffect(()=>{if(!compareOpen)return;const ctrl=new AbortController();setCompare(null);setModalError('');fetch('/api/v1/compare?offer_ids='+selected.join(','),{signal:ctrl.signal}).then(r=>{if(!r.ok)throw Error('Не удалось загрузить сравнение');return r.json()}).then(items=>{if(!ctrl.signal.aborted)setCompare(items)}).catch(()=>{if(!ctrl.signal.aborted)setModalError('Не удалось загрузить сравнение. Попробуйте открыть его ещё раз.')});return()=>ctrl.abort()},[compareOpen,selected]);
 function navigate(raw:URLSearchParams,preserveDraft=false){const p=canonicalParams(raw),query=p.toString();history.pushState(null,'',resultsPath+(query?'?'+query:''));setParams(p);if(!preserveDraft)setFormRevision(v=>v+1)}
 function search(e:React.FormEvent<HTMLFormElement>){e.preventDefault();const form=e.currentTarget,f=new FormData(form),p=new URLSearchParams();const min=String(f.get('min')||''),max=String(f.get('max')||'');if(min&&max&&Number(min)>Number(max)){const input=form.elements.namedItem('max') as HTMLInputElement;input.setCustomValidity('Максимальный бюджет должен быть не меньше минимального.');input.reportValidity();return;}f.forEach((v,k)=>{if(v){p.set(k,k==='min'||k==='max'?String(Math.round(Number(v)*100)):String(v))}});focusResults.current=true;navigate(p)}
 function updateSelection(ids:string[]){setSelected(ids);try{localStorage.setItem('devcourse-offers',JSON.stringify(ids))}catch{}}
 function toggle(id:string){setNotice('');if(selected.includes(id)){updateSelection(selected.filter(x=>x!==id));return;}if(selected.length===3){setNotice('В сравнении уже 3 тарифа. Уберите один, чтобы добавить новый.');return;}updateSelection([...selected,id])}
 function removeComparison(id:string){const ids=selected.filter(x=>x!==id);updateSelection(ids);if(!ids.length){close();return;}const p=new URLSearchParams(location.search);p.set('compare',ids.join(','));history.replaceState(null,'',location.pathname+'?'+p);}
 function showCompare(){try{localStorage.setItem('devcourse-offers',JSON.stringify(selected))}catch{};location.assign('/compare?offers='+selected.map(encodeURIComponent).join(','))}
 function close(){setCompareOpen(false);setModalError('');const p=new URLSearchParams(location.search);p.delete('compare');history.replaceState(null,'',location.pathname+(p.toString()?'?'+p:''));setParams(p)}
 useEffect(()=>{if(new URLSearchParams(location.search).has('compare'))return;try{const saved=JSON.parse(localStorage.getItem('devcourse-offers')||'[]');if(Array.isArray(saved))setSelected([...new Set(saved.filter((v:unknown):v is string=>typeof v==='string'))].slice(0,3))}catch{}},[]);
 const searchButton=useRef<HTMLButtonElement>(null);
 const resultHeading=useRef<HTMLHeadingElement>(null), focusResults=useRef(false);
 useEffect(()=>{if(!loading&&focusResults.current){focusResults.current=false;resultHeading.current?.focus({preventScroll:true});resultHeading.current?.scrollIntoView({block:'start',behavior:matchMedia('(prefers-reduced-motion: reduce)').matches?'instant':'smooth'});}},[loading,data,error]);
 const items=data?.items||[],page=data?.page||1;
 const confirmedItems=items.filter(i=>i.profile_match!=="unverified"),unverifiedItems=items.filter(i=>i.profile_match==="unverified");
 const confirmedTotal=data?.confirmed_total??data?.total??0;
 const hasProfileFilters=['experience','goal','direction'].some(k=>params?.has(k));
 const counts=!loading&&!error?data?.diagnostics:undefined;
 function includeUnknown(){const p=new URLSearchParams(params||'');p.set('include_unknown_profiles','true');p.delete('page');navigate(p)}
 const hasFilters=Array.from(params?.keys()||[]).some(key=>!['sort','page','page_size','compare'].includes(key));
 return <>
 <main><section className="intro glassHero"><div className="heroCopy"><h1>Найдите<br/>свой <span>курс.</span></h1><p>Сравните курсы по опыту,<br/>цели и бюджету.</p></div><div className="languageScene" role="group" aria-label="Выберите язык разработки"><span className="sceneCaption">С чего начнём?</span>{Object.entries(languages).map(([key,name])=><button key={key} type="button" aria-pressed={params?.get('language')===key} onClick={()=>{const p=new URLSearchParams(params||'');p.set('language',key);p.delete('page');navigate(p);document.getElementById('catalog-filters')?.scrollIntoView({behavior:window.matchMedia('(prefers-reduced-motion: reduce)').matches?'instant':'smooth'})}}><span className="languageCode" aria-hidden="true">{{python:'Py',javascript:'JS',java:'Ja',go:'Go'}[key]}</span>{name}</button>)}<span className="sceneFoot">Ваш следующий шаг в разработку</span></div></section>
 <div className="workspace"><aside id="catalog-filters" aria-label="Ваш маршрут"><div className="filterTitle"><h2>Ваш маршрут</h2><button className="textButton" onClick={()=>navigate(new URLSearchParams())}>Сбросить</button></div>
 {params&&<form key={formRevision} onSubmit={search} onChange={e=>{const form=e.currentTarget;const input=form.elements.namedItem('max') as HTMLInputElement;input.setCustomValidity('')}}>
 <fieldset><legend>Язык разработки</legend><div className="languageGrid"><label className="languageChoice"><input type="radio" name="language" value="" defaultChecked={!params.get('language')}/><span>Все языки</span></label>{Object.entries(languages).map(([k,v])=><label className="languageChoice" key={k}><input type="radio" name="language" value={k} defaultChecked={params.get('language')===k}/><span>{v}</span></label>)}</div></fieldset>
 <Select label="Текущий опыт" name="experience" values={experience} counts={counts?.experience.counts} defaultValue={params.get('experience')||''}/>
 <Select label="Ваша цель" name="goal" values={goals} counts={counts?.goal.counts} defaultValue={params.get('goal')||''}/>
 <Select label="Стоимость" name="budget" values={{free:'Только бесплатно',paid:'Платные программы'}} defaultValue={params.get('budget')||''}/>
 <div className="priceInputs"><label>От, ₽<input name="min" type="number" min="0" max="100000000" placeholder="0" defaultValue={params.has('min')?Number(params.get('min'))/100:''}/></label><label>До, ₽<input name="max" type="number" min="0" max="100000000" placeholder="Без лимита" defaultValue={params.has('max')?Number(params.get('max'))/100:''}/></label></div><small>Полная стоимость обучения, не платёж в месяц.</small>
 <label className="checkbox"><input type="checkbox" name="include_free" value="true" defaultChecked={params.get('include_free')==='true'}/>Добавить бесплатные к платным</label>
 <details><summary>Больше параметров <span>+</span></summary><Select label="Направление" name="direction" values={directions} counts={counts?.direction.counts} defaultValue={params.get('direction')||''}/><Select label="Обратная связь" name="support" values={{self:'Самостоятельно',review:'Проверка заданий',mentor:'Наставник'}} defaultValue={params.get('support')||''}/><Select label="Расписание" name="schedule" values={{flexible:'В своём темпе',scheduled:'По расписанию'}} defaultValue={params.get('schedule')||''}/><label>Часов в неделю, не больше<input name="hours" type="number" min="1" max="168" defaultValue={params.get('hours')||''}/></label><label className="checkbox"><input type="checkbox" name="include_closed" value="true" defaultChecked={params.get('include_closed')==='true'}/>Показать закрытый и неизвестный набор</label></details>
 <div className="profileOptions">{counts&&<small className="profileCountHelp">Числа — программы с подтверждённым параметром при остальных выбранных фильтрах.</small>}
 <label className="checkbox"><input type="checkbox" name="include_unknown_profiles" value="true" defaultChecked={params.get('include_unknown_profiles')==='true'}/>Показать курсы с неполными данными</label></div>
 <input type="hidden" name="sort" value={params.get('sort')||'recent'}/><button ref={searchButton} className="primary searchButton" type="submit">Найти обучение <span>→</span></button></form>}</aside>
 <div className="srOnly" role="status" aria-live="polite" aria-atomic="true">{announcement}</div>
 <section id="catalog" className="results" aria-label="Результаты поиска" aria-busy={loading}><div className="resultsHeading"><div className="resultTitle"><div className="eyebrow">КАТАЛОГ ОБУЧЕНИЯ</div><h2 ref={resultHeading} tabIndex={-1} className="resultHeading">{loading?'Ищем подходящие программы…':error?'Поиск недоступен':confirmedTotal?countLabel(confirmedTotal)+(hasProfileFilters?' с подтверждёнными условиями':hasFilters?' по вашим условиям':' в каталоге'):hasProfileFilters?'Подтверждённых совпадений нет':hasFilters?'Пока без совпадений':'Каталог пока пуст'}</h2></div>
 <div className="filterChips" role="group" aria-label="Применённые фильтры">{Array.from(params?.entries()||[]).filter(([key])=>key in filterLabels).map(([key,value])=><button className="filterChip" key={key} aria-label={"Убрать фильтр: "+filterLabels[key]} onClick={()=>{const p=new URLSearchParams(params||'');p.delete(key);p.delete('page');navigate(p)}}>{filterLabels[key]}: {filterValue(key,value)} <span aria-hidden="true">×</span></button>)}</div>
 <div className="sort"><select aria-label="Сортировка" value={params?.get('sort')||'recent'} onChange={e=>{const p=new URLSearchParams(params||'');p.set('sort',e.target.value);p.delete('page');navigate(p,true)}}><option value="recent">Сначала свежие данные</option><option value="price_asc">Сначала дешевле</option><option value="price_desc">Сначала дороже</option><option value="duration">Сначала короче</option></select></div></div>
 {!loading&&!error&&hasProfileFilters&&(data?.unknown_available??0)>0&&params?.get('include_unknown_profiles')!=='true'&&<div className="profileNotice"><p>Ещё {countLabel(data?.unknown_available??0)} с неполными данными. Соответствие выбранному опыту, цели или направлению не подтверждено. Эти курсы не входят в совпадения.</p><button type="button" onClick={includeUnknown}>Показать курсы с неполными данными</button></div>}
 {items.some(i=>i.course.demo)&&<div className="demoBanner"><strong>Демонстрационный каталог.</strong> Названия и цены — примеры для проверки сайта, не реальные предложения школ.</div>}
 {notice&&<div role="status" className="demoBanner">{notice}</div>}
 {loading?<div className="skeletons" role="status" aria-label="Загрузка">{[1,2,3].map(n=><div key={n} className="skeleton"/>)}</div>:error?<ResultState kind="error" message={error} onRetry={()=>setRetry(x=>x+1)} onReset={hasFilters?()=>navigate(new URLSearchParams()):undefined}/>:!items.length?<ResultState kind={hasFilters?"filtered":"empty"} onRetry={()=>setRetry(x=>x+1)} onReset={()=>navigate(new URLSearchParams())}/>:<><div className="cards">{confirmedItems.map(i=><CourseCard key={i.offer.id} i={i} selected={selected.includes(i.offer.id)} onToggle={toggle}/>)}</div>{unverifiedItems.length>0&&<section className="unverifiedResults" aria-labelledby="unverified-heading"><h3 id="unverified-heading">Условия нужно уточнить</h3><p>Не удалось подтвердить соответствие выбранному опыту, цели или направлению. Уточните условия у школы.</p><div className="cards">{unverifiedItems.map(i=><CourseCard key={i.offer.id} i={i} selected={selected.includes(i.offer.id)} onToggle={toggle}/>)}</div></section>}</>}
 {!loading&&!error&&data&&data.total>data.page_size&&<nav className="pagination" aria-label="Страницы"><button disabled={page===1} onClick={()=>{const p=new URLSearchParams(params||'');p.set('page',String(page-1));navigate(p,true)}}>← Назад</button><span>{page} / {Math.ceil(data.total/data.page_size)}</span><button disabled={page*data.page_size>=data.total} onClick={()=>{const p=new URLSearchParams(params||'');p.set('page',String(page+1));navigate(p,true)}}>Далее →</button></nav>}
 </section></div>
 <section id="how" className="how"><div><div className="eyebrow">ПОНЯТНЫЙ ВЫБОР</div><h2>Условия важнее<br/><span>обещаний.</span></h2></div><p>Мы сравниваем конкретные тарифы и полную стоимость. Если цена устарела или неизвестна, показываем это явно и не включаем программу в поиск с ограничением бюджета. Переход и запись — на сайте источника. Каталог не гарантирует трудоустройство.</p></section></main>
 {selected.length>0&&!compareOpen&&<div className="compareBar"><span><strong>{selected.length} из 3</strong> тарифов для сравнения</span><button className="primary" onClick={showCompare}>Сравнить →</button><button className="textButton" onClick={()=>updateSelection([])}>Очистить</button></div>}
 {(compareOpen||modalError)&&<Modal label="Сравнение тарифов" onClose={close} fallbackFocus={searchButton}>
 {modalError?<p role="alert">{modalError}</p>:<><div className="eyebrow">ВАШ КОРОТКИЙ СПИСОК</div><h2>Сравнение тарифов</h2><p>Одинаковые критерии — осознанный выбор. Ссылка на эту страницу сохраняет сравнение.</p>{!compare?<p role="status">Загрузка…</p>:<ComparisonTable items={compare} ids={selected} onRemove={removeComparison}/>}</>}
 </Modal>}</>
}
