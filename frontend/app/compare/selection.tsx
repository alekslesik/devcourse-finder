 'use client';
import {useEffect} from 'react';
export default function Selection({ids}:{ids:string[]}){useEffect(()=>{try{localStorage.setItem('devcourse-offers',JSON.stringify(ids))}catch{}},[ids]);return null}
