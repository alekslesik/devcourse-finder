import {languageMarks} from '../lib/language-marks';
export default function LanguageIcon({language}:{language:string}) {
 const mark=languageMarks[language];
 return mark?<svg className="languageIcon" viewBox={mark.viewBox} aria-hidden="true" focusable="false" dangerouslySetInnerHTML={{__html:mark.markup}}/>:null;
}
