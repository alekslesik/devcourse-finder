import type {Metadata} from 'next';
import CoursePassport from '../../course-passport';
import {socialMetadata} from '../../../lib/social-metadata';
import {course} from '../../../lib/api';
type Props={params:Promise<{slug:string}>;searchParams?:Promise<{offer?:string|string[]}>};
export const dynamic='force-dynamic';
export async function generateMetadata({params}:Props):Promise<Metadata>{
 const {slug}=await params;const d=await course(slug);
 return socialMetadata(d.course.title+' — DevCourseFinder',d.course.summary,'/courses/'+encodeURIComponent(slug));
}
export default async function CoursePage({params,searchParams}:Props){
 const d=await course((await params).slug);
 const selected=(await searchParams)?.offer;
 const initialOfferId=typeof selected==='string'&&d.offers.some(i=>i.offer.id===selected)?selected:undefined;
 // Avoid serializing a full repeated Course object for every Result into the client view.
 const c=d.course;
 const summary={title:c.title,provider:c.provider,language:c.language,summary:c.summary,audience:c.audience,profile:c.profile,topics:c.topics,source:c.source,checked_at:c.checked_at,demo:c.demo};
 const offers=d.offers.map(({offer,effective_price})=>({offer,effective_price}));
 return <main className="document coursePage"><CoursePassport course={summary} offers={offers} initialOfferId={initialOfferId}/></main>;
}
