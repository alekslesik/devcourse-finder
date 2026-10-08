import {logoMarkup,logoViewBox} from '../lib/brand';
export default function Brand() {
  return <a className="brand" href="/" aria-label="DevCourseFinder — на главную"><svg viewBox={logoViewBox} aria-hidden="true" focusable="false" dangerouslySetInnerHTML={{__html:logoMarkup}}/></a>;
}
