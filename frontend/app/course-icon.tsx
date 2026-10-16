import type {ReactNode} from 'react';
const marks={
 clock:<><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></>,
 calendar:<><rect x="3" y="5" width="18" height="16" rx="3"/><path d="M7 3v4m10-4v4M3 11h18m-13 4h2m4 0h2"/></>,
 person:<><circle cx="12" cy="7" r="4"/><path d="M4 21v-2a8 8 0 0 1 16 0v2"/></>,
 people:<><circle cx="9" cy="7" r="3"/><path d="M2 21v-3a7 7 0 0 1 14 0v3m0-17a3 3 0 0 1 0 6m3 4a6 6 0 0 1 3 5v2"/></>,
 code:<path d="m8 5-7 7 7 7m8-14 7 7-7 7"/>,
 book:<><path d="M12 5C9 2 4 2 2 3v16c3-1 7-1 10 2 3-3 7-3 10-2V3c-2-1-7-1-10 2Z"/><path d="M12 5v16"/></>,
 link:<><path d="m10 13 4-4m-6 7-1 1a4 4 0 0 1-6-6l4-4a4 4 0 0 1 6 0m2 1 1-1a4 4 0 0 1 6 6l-4 4a4 4 0 0 1-6 0"/></>,
 unknown:<><circle cx="12" cy="12" r="9"/><path d="M8 12h8"/></>,
} satisfies Record<string,ReactNode>;
export default function CourseIcon({kind}:{kind:keyof typeof marks}){
 return <svg className="courseIcon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">{marks[kind]}</svg>;
}
