import type {Course} from '../lib/catalog';
import RecommendationEvidence from './recommendation-evidence';
const labels:Record<string,Record<string,string>>={
 audience:{none:'Начинающим с нуля',basics:'Тем, кто знает основы',projects:'Тем, кто уже пишет проекты',working:'Работающим разработчикам',switch:'Программистам с другим языком'},
 goals:{try:'Попробовать программирование',job:'Подготовиться к первой работе',switch:'Сменить язык или направление',deepen:'Углубить знания'},
 directions:{basics:'Основы',backend:'Backend',frontend:'Frontend',fullstack:'Fullstack',automation:'Автоматизация',data:'Данные и машинное обучение',algorithms:'Алгоритмы',testing:'Тестирование'}
};
export default function RecommendationOverview({course}:{course:Pick<Course,'source'|'recommendations'>}){
 const r=course.recommendations;if(!r)return null;
 const entries=[{key:'audience',field:'experience',name:'Кому может подойти',tags:r.audience},{key:'goals',field:'goal',name:'Для какой задачи',tags:r.goals},{key:'directions',field:'direction',name:'Направление',tags:r.directions}].filter(e=>e.tags&&Date.now()-Date.parse(e.tags.evidence.checked_at)<=90*24*60*60*1000);
 if(!entries.length)return null;
 return <div className="recommendationOverview"><h3>Оценка ИИ по программе</h3><p>Помогает с выбором, но не заменяет подтверждённые требования школы и не обещает трудоустройство.</p><dl>{entries.map(e=><div key={e.key}><dt>{e.name}</dt><dd>{e.tags!.values.map(v=>labels[e.key][v]||v).join(' · ')}</dd></div>)}</dl><RecommendationEvidence course={course} fields={entries.map(e=>e.field)} summary="На чём основана оценка ИИ"/></div>
}
