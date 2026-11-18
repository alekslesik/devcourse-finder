'use client';
import {useState,type ReactNode} from 'react';

export default function AdvancedFilters({initialOpen,children}:{initialOpen:boolean;children:ReactNode}){
 const [open,setOpen]=useState(initialOpen);
 return <details className="advancedFilters" open={open} onToggle={e=>setOpen(e.currentTarget.open)}><summary>Дополнительные фильтры</summary><div className="advancedFields">{children}</div></details>;
}
