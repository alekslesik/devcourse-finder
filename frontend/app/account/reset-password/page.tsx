import type {Metadata} from 'next';
import {notFound} from 'next/navigation';
import AccountActionForm from '../action-form';
export const dynamic='force-dynamic';
export const metadata:Metadata={title:'Восстановить доступ — DevCourseFinder',robots:{index:false,follow:false}};
export default function ResetPassword(){if(process.env.AUTH_ACTIONS_ENABLED!=='true')notFound();return <AccountActionForm kind="reset"/>;}
