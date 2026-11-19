'use client';
import {usePathname} from 'next/navigation';
import {isAccountPath} from '../lib/account-privacy';
import {metrikaWatchURL} from '../lib/metrika';
export default function NoScriptAnalytics(){const path=usePathname();if(isAccountPath(path))return null;return <noscript><div><img src={metrikaWatchURL} style={{position:'absolute',left:'-9999px'}} alt=""/></div></noscript>;}
