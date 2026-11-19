'use client';
import {useEffect,useRef,useState,type FormEvent} from 'react';

declare global{interface Window{__dcfAccountEntry?:{token:string}}}
type Kind='confirm'|'reset';
type Reply={code?:string;message?:string};
async function submitAction(action:string,data:Record<string,string>):Promise<{status:number;data:Reply}>{
 const response=await fetch('/api/v1/account/csrf',{cache:'no-store',credentials:'same-origin'});
 if(!response.ok)throw Error('unavailable');const csrf=await response.json() as {token?:string};
 if(!csrf.token||!/^v1\.[A-Za-z0-9_-]{43}$/.test(csrf.token))throw Error('unavailable');
 const result=await fetch('/api/v1/account/'+action,{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':csrf.token},credentials:'same-origin',cache:'no-store',body:JSON.stringify(data)});
 return {status:result.status,data:await result.json() as Reply};
}
export default function AccountActionForm({kind}:{kind:Kind}){
 const [ready,setReady]=useState(false),[token,setToken]=useState(''),[email,setEmail]=useState(''),[password,setPassword]=useState(''),[repeat,setRepeat]=useState('');
 const [pending,setPending]=useState(false),[complete,setComplete]=useState(false),[sent,setSent]=useState(false),[message,setMessage]=useState(''),[field,setField]=useState(''),[visible,setVisible]=useState(false),[cooldown,setCooldown]=useState(0);
 const initialized=useRef(false),inFlight=useRef(false),emailRef=useRef<HTMLInputElement>(null),passwordRef=useRef<HTMLInputElement>(null),repeatRef=useRef<HTMLInputElement>(null),statusRef=useRef<HTMLDivElement>(null);
 useEffect(()=>{if(initialized.current)return;initialized.current=true;const entry=window.__dcfAccountEntry;if(!entry){location.replace(location.href);return;}setToken(entry.token);delete window.__dcfAccountEntry;setReady(true);},[]);
 useEffect(()=>{if(!cooldown)return;const timer=setTimeout(()=>setCooldown(value=>Math.max(0,value-1)),1000);return()=>clearTimeout(timer);},[cooldown]);
 useEffect(()=>{if(complete||sent)statusRef.current?.focus();},[complete,sent]);
 const valid=token!=='';
 function invalid(text:string,target:string){setMessage(text);setField(target);({email:emailRef,password:passwordRef,repeat:repeatRef}[target])?.current?.focus();}
 async function submit(event:FormEvent<HTMLFormElement>){
  event.preventDefault();if(inFlight.current||complete)return;setMessage('');setField('');
  if(!valid&&(!emailRef.current?.validity.valid||!email)){invalid('Введите email в формате name@example.com.','email');return;}
  if(valid&&kind==='reset'){
   if(Array.from(password).length<15||new TextEncoder().encode(password).length>1024){invalid('Нужно не менее 15 символов и не более 1024 байт.','password');return;}
   if(password!==repeat){invalid('Пароли не совпадают. Повторите новый пароль.','repeat');return;}
  }
  inFlight.current=true;setPending(true);
  try{
   const action=valid?(kind==='confirm'?'confirm-email':'reset-password'):(kind==='confirm'?'request-confirmation':'request-reset');
   const input:Record<string,string>=valid?(kind==='confirm'?{token}:{token,password}):{email};
   const result=await submitAction(action,input);
   if(result.status===202){setSent(true);setCooldown(60);}
   else if(result.status===200){setComplete(true);setToken('');setPassword('');setRepeat('');}
   else if(result.data.code==='invalid_link'){setToken('');setPassword('');setRepeat('');setMessage('Ссылка недействительна или устарела. Запросите новое письмо.');}
   else if(result.data.code==='password_common'||result.data.code==='password_policy'){invalid(result.data.code==='password_common'?'Выберите менее распространённый пароль.':'Нужно не менее 15 символов и не более 1024 байт.','password');}
   else setMessage(result.status===429?'Слишком много попыток. Подождите немного и повторите.':'Не удалось выполнить действие. Попробуйте ещё раз позже.');
  }catch{setMessage('Не удалось связаться с сервисом. Проверьте соединение и повторите.');}
  finally{inFlight.current=false;setPending(false);}
 }
 const title=complete?(kind==='confirm'?'Email подтверждён':'Пароль изменён'):kind==='confirm'?'Подтвердить email':valid?'Новый пароль':'Восстановить доступ';
 return <main className="accountFlow"><h1>{title}</h1>
  {!ready?<p role="status">Загрузка формы…</p>:complete?<div ref={statusRef} tabIndex={-1} role="status"><p>{kind==='confirm'?'Теперь этот адрес можно использовать для восстановления доступа.':'Старый пароль больше не действует.'}</p><a className="linkButton" href="/">Вернуться к курсам</a></div>:<>
   <p className="accountIntro">{valid?(kind==='confirm'?'Нажмите кнопку, чтобы завершить подтверждение адреса.':'Задайте пароль, который не используете на других сайтах.'):(kind==='confirm'?'Укажите email, чтобы получить новое письмо подтверждения.':'Укажите email, который вы подтверждали в сервисе.')}</p>
   {sent&&<div ref={statusRef} tabIndex={-1} role="status" className="accountNotice"><p>Если адрес подходит для этого действия, письмо придёт в ближайшее время. Проверьте папку «Спам».</p></div>}
   {message&&<p id="account-error" role="alert" className="accountError">{message}</p>}
   <form onSubmit={submit} noValidate aria-busy={pending}>
    {!valid?<label htmlFor="account-email">Email<input ref={emailRef} id="account-email" type="email" name="email" inputMode="email" autoComplete="email" autoCapitalize="none" spellCheck={false} maxLength={254} required value={email} onChange={e=>setEmail(e.target.value)} disabled={pending} aria-invalid={field==='email'||undefined} aria-describedby={field==='email'?'account-error':undefined}/></label>:kind==='reset'?<>
     <label htmlFor="account-password">Новый пароль</label><div className="accountPassword"><input ref={passwordRef} id="account-password" name="password" type={visible?'text':'password'} autoComplete="new-password" minLength={15} maxLength={1024} required value={password} onChange={e=>setPassword(e.target.value)} disabled={pending} aria-invalid={field==='password'||undefined} aria-describedby={field==='password'?'account-error account-password-hint':'account-password-hint'}/><button type="button" className="textButton" aria-label={visible?'Скрыть пароль':'Показать пароль'} aria-pressed={visible} onClick={()=>setVisible(!visible)} disabled={pending}>{visible?'Скрыть':'Показать'}</button></div>
     <p id="account-password-hint" className="accountHint">Не менее 15 символов. Можно использовать пробелы и вставлять пароль из менеджера.</p>
     <label htmlFor="account-repeat">Повторите пароль<input ref={repeatRef} id="account-repeat" name="password_confirmation" type={visible?'text':'password'} autoComplete="new-password" maxLength={1024} required value={repeat} onChange={e=>setRepeat(e.target.value)} disabled={pending} aria-invalid={field==='repeat'||undefined} aria-describedby={field==='repeat'?'account-error':undefined}/></label>
    </>:null}
    <button className="accountSubmit" type="submit" disabled={pending||(!valid&&cooldown>0)}>{pending?'Отправляем…':!valid?(cooldown?`Отправить ещё раз через ${cooldown} с`:'Отправить письмо'):kind==='confirm'?'Подтвердить email':'Сохранить новый пароль'}</button>
   </form>
   <p className="accountBack"><a href="/">Вернуться к курсам</a></p>
  </>}
 </main>;
}
