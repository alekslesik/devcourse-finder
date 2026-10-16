// Presentation-only detail fixtures; never inserted into catalog search results.
export function passportCourses(template){
 const course=structuredClone(template);
 course.id=course.slug='passport-multi';course.title='Демо: Проверка паспорта курса';
 course.provider='Яндекс Практикум';course.audience=['unknown'];course.topics=[];
 course.audience=['none','switch'];course.profile={prerequisites:{level:'basics',scope:'course_language',evidence:{source:course.source,locator:'test prerequisites',digest:'test',checked_at:course.checked_at}}};
 const base={...course.offers[0],learning_conditions:undefined,duration_months:3,weeks:null,hours:null,price:7000000,price_kind:'exact',free:false,support_known:false,review:false,mentor:false,schedule:'scheduled',enrollment:'open'};
 course.offers=[
  {...base,id:'passport-standard',name:'Полная программа'},
  {...base,id:'passport-free',name:'Вводный тариф',price:0,free:true,duration_months:null,weeks:1,hours:2,support_known:true,schedule:'flexible',enrollment:'continuous'},
  {...base,id:'passport-from',name:'Тариф с неподтверждённой ценой',price:3000000,price_kind:'from',duration_months:null,enrollment:'unknown'},
  {...base,id:'passport-closed',name:'Закрытый тариф',price:3500000,duration_months:6,review:true,support_known:true,enrollment:'closed'},
 ];
 const empty=structuredClone(course);empty.id=empty.slug='passport-empty';empty.offers=[];empty.audience=['unknown'];empty.profile=undefined;
 return [course,empty];
}
