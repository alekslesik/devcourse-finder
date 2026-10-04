'use client';
import {useLayoutEffect,useRef} from 'react';
import type {ReactNode,RefObject} from 'react';

export default function Modal({label,onClose,fallbackFocus,children}:{label:string;onClose:()=>void;fallbackFocus:RefObject<HTMLElement|null>;children:ReactNode}){
 const ref=useRef<HTMLDialogElement>(null);
 useLayoutEffect(()=>{
  const dialog=ref.current!;
  const previous=document.activeElement instanceof HTMLElement?document.activeElement:null;
  const overflow=document.body.style.overflow;
  dialog.showModal();
  document.body.style.overflow='hidden';
  return()=>{
   dialog.close();
   document.body.style.overflow=overflow;
   const target=previous&&previous!==document.body&&previous.isConnected?previous:fallbackFocus.current;
   target?.focus({preventScroll:true});
  };
 },[fallbackFocus]);
 return <dialog ref={ref} className="overlay" aria-label={label} onCancel={e=>{e.preventDefault();onClose()}} onClick={e=>{if(e.target===e.currentTarget)onClose()}} onKeyDown={e=>{
  if(e.key!=='Tab')return;
  const nodes=Array.from(e.currentTarget.querySelectorAll<HTMLElement>('a[href],button,input,select,textarea,[tabindex]')).filter(node=>node.tabIndex>=0&&!node.matches(':disabled')&&node.getClientRects().length>0);
  const first=nodes[0],last=nodes[nodes.length-1];
  if(!first){e.preventDefault();e.currentTarget.focus();return}
  if(e.shiftKey&&document.activeElement===first){e.preventDefault();last.focus()}
  else if(!e.shiftKey&&document.activeElement===last){e.preventDefault();first.focus()}
 }}><section className="modal"><button className="close" aria-label="Закрыть" onClick={onClose}>×</button>{children}</section></dialog>;
}
