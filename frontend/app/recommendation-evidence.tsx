import type {Course} from '../lib/catalog';
import ExternalMark from './external-mark';
const names:Record<string,string>={experience:'Подходящий опыт',goal:'Цель обучения',direction:'Направление'};
export default function RecommendationEvidence({course,fields=[],summary='Почему ИИ предлагает этот курс'}:{course:Pick<Course,'source'|'recommendations'>;fields?:string[];summary?:string}){
 const profile=course.recommendations;
 type Tags=NonNullable<NonNullable<Course['recommendations']>['audience']>;
 const groups=new Map<string,{fields:string[];tags:Tags}>();
 for(const field of fields){
  const tags=field==='experience'?profile?.audience:field==='goal'?profile?.goals:profile?.directions;
  if(!tags)continue;
  const key=tags.evidence.source+'\n'+tags.quote;
  const existing=groups.get(key);if(existing)existing.fields.push(field);else groups.set(key,{fields:[field],tags});
 }
 if(!groups.size)return null;
 return <details className="recommendationEvidence"><summary>{summary}</summary><p>Это оценка содержания, а не подтверждённое условие школы. Сверьте программу со своей задачей.</p>{Array.from(groups.entries()).map(([key,{fields,tags}])=><div key={key}><strong>{fields.map(field=>names[field]).join(' · ')}</strong><blockquote>{tags.quote}</blockquote><a href={course.source} target="_blank" rel="noopener noreferrer">Программа курса <ExternalMark/></a><small>Оценка от {new Date(tags.evidence.checked_at).toLocaleDateString('ru-RU',{timeZone:'Europe/Moscow'})}</small></div>)}</details>
}
