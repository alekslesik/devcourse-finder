import type {Metadata} from 'next';
import {notFound} from 'next/navigation';
import AccountActionForm from '../action-form';
export const dynamic='force-dynamic';
export const metadata:Metadata={title:'Подтвердить email — DevCourseFinder',robots:{index:false,follow:false}};
export default function ConfirmEmail(){if(process.env.AUTH_ACTIONS_ENABLED!=='true')notFound();return <AccountActionForm kind="confirm"/>;}
