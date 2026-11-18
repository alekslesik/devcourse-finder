'use client';
import {useEffect,useRef} from 'react';
import {recordEvent} from '../lib/analytics';

// Count after successful rendering, never during server prefetch or on errors.
// Replayed React effects do not count the same rendered page twice.
export default function ActivityEvent({kind,courseId='',language='',total=0,identity=''}:{kind:'view'|'compare';courseId?:string;language?:string;total?:number;identity?:string}){
 const sent=useRef('');
 useEffect(()=>{
  const key=JSON.stringify([kind,courseId,language,total,identity]);
  if(sent.current===key)return;
  sent.current=key;
  recordEvent(kind,kind==='view'?{course_id:courseId,language}:{total});
 },[kind,courseId,language,total,identity]);
 return null;
}
