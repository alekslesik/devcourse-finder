import type {Metadata} from 'next';
import {socialMetadata} from '../../lib/social-metadata';
import Finder from '../finder';

export const metadata:Metadata={
 ...socialMetadata('Каталог курсов для разработчиков — DevCourse','Подберите обучение Go, Python, Java или JavaScript по опыту, цели, бюджету и формату поддержки.','/courses'),
 robots:{index:false,follow:true},
};

export default function CoursesPage(){
 return <Finder resultsPath="/courses"/>;
}
