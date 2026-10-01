package catalog
import("fmt";"net/url";"strconv";"slices")
func Parse(q url.Values)(Filter,error){
 f:=Filter{Language:q.Get("language"),Direction:q.Get("direction"),Experience:q.Get("experience"),Goal:q.Get("goal"),Budget:q.Get("budget"),Support:q.Get("support"),Schedule:q.Get("schedule"),Sort:q.Get("sort"),Page:1,PageSize:12}
 enums:=map[string][]string{"language":{"go","python","java","javascript"},"direction":{"basics","backend","frontend","fullstack","automation"},"experience":{"none","basics","projects","working","switch","experienced"},"goal":{"try","job","switch","deepen"},"budget":{"free","paid"},"support":{"self","review","mentor"},"schedule":{"flexible","scheduled"},"sort":{"recent","price_asc","price_desc","duration"}}
 for k,allowed:=range enums{if v:=q.Get(k);v!=""&&!slices.Contains(allowed,v){return f,fmt.Errorf("invalid %s",k)}}
 for _,k:=range []string{"min","max","hours","page","page_size"}{v:=q.Get(k);if v==""{continue};n,e:=strconv.ParseInt(v,10,64);if e!=nil||n<0||n>10000000000{return f,fmt.Errorf("invalid %s",k)}
 switch k{case "min":f.Min=&n;case "max":f.Max=&n;case "hours":if n<1||n>168{return f,fmt.Errorf("invalid hours")};h:=int(n);f.Hours=&h;case "page":if n<1||n>1000000{return f,fmt.Errorf("invalid page")};f.Page=int(n);case "page_size":if n<1||n>48{return f,fmt.Errorf("invalid page_size")};f.PageSize=int(n)}
 }
 for _,k:=range []string{"include_free","include_closed"}{v:=q.Get(k);if v!=""&&v!="true"&&v!="false"{return f,fmt.Errorf("invalid %s",k)}}
 f.IncludeFree=q.Get("include_free")=="true";f.IncludeClosed=q.Get("include_closed")=="true"
 if f.Min!=nil&&f.Max!=nil&&*f.Min>*f.Max{return f,fmt.Errorf("min exceeds max")};return f,nil
}
