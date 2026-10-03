 'use client';
import {useState} from 'react';
export default function CatalogActions({id}:{id:string}){
 const [message,setMessage]=useState('');
 function add(){let ids:string[]=[];try{ids=JSON.parse(localStorage.getItem('devcourse-offers')||'[]').filter((v:unknown)=>typeof v==='string')}catch{}
 if(!ids.includes(id)&&ids.length>=3){setMessage('В сравнении уже 3 тарифа. Удалите один перед добавлением.');return}
 ids=[...new Set([...ids,id])];try{localStorage.setItem('devcourse-offers',JSON.stringify(ids))}catch{}
 location.assign('/compare?offers='+ids.map(encodeURIComponent).join(','));}
 return <><button className="secondary" onClick={add}>Добавить в сравнение</button><p role="status">{message}</p></>;
}
