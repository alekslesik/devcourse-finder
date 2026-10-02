import type {Metadata} from 'next';
import Finder from '../finder';

export const metadata:Metadata={
 title:'Каталог курсов для разработчиков — DevCourse',
 description:'Подберите обучение Go, Python, Java или JavaScript по опыту, цели, бюджету и формату поддержки.',
 alternates:{canonical:'/courses'},
 robots:{index:false,follow:true},
};

export default function CoursesPage(){
 return <Finder resultsPath="/courses"/>;
}
