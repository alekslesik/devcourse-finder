// Match published display names, never course IDs or untrusted source URLs.
const providers:Record<string,string>={
 'javarush':'javarush', 'skypro':'skypro', 'skillfactory':'skillfactory',
 'skillbox':'skillbox', 'html academy':'htmlacademy', 'rs school':'rsschool',
 'purpleschool':'purpleschool', 'нетология':'netology', 'stepik':'stepik',
 'otus':'otus', 'яндекс практикум':'yandex', 'хекслет':'hexlet', 'codebasics':'codebasics',
};

export default function ProviderName({name}:{name:string}){
 return <span className="provider" data-provider={providers[name.trim().toLocaleLowerCase('ru-RU')]}>{name}</span>;
}
