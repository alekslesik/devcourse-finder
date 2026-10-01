package catalog

import ("time"; "sort"; "slices")
type Course struct {
 ID string `json:"id"`; Slug string `json:"slug"`; Title string `json:"title"`; Provider string `json:"provider"`
 Language string `json:"language"`; Direction string `json:"direction"`; Summary string `json:"summary"`
 Audience []string `json:"audience"`; Goals []string `json:"goals"`; Topics []string `json:"topics"`
 Source string `json:"source"`; CheckedAt time.Time `json:"checked_at"`; Status string `json:"status"`
 Offers []Offer `json:"offers"`; Demo bool `json:"demo"`
}
type Offer struct {
 ID string `json:"id"`; Name string `json:"name"`; Price *int64 `json:"price"`; PriceKind string `json:"price_kind"`; Free bool `json:"free"`
 PriceCheckedAt time.Time `json:"price_checked_at"`; ValidUntil *time.Time `json:"valid_until,omitempty"`
 Hours *int `json:"hours"`; Weeks *int `json:"weeks"`; Review bool `json:"review"`; Mentor bool `json:"mentor"`
 Schedule string `json:"schedule"`; Enrollment string `json:"enrollment"`; URL string `json:"url"`
}
type Filter struct { Language, Direction, Experience, Goal, Budget, Support, Schedule, Sort string; Min,Max *int64; Hours *int; IncludeFree, IncludeClosed bool; Page, PageSize int }
type Result struct { Course Course `json:"course"`; Offer Offer `json:"offer"`; Price *int64 `json:"effective_price"`; Reasons []string `json:"reasons"`; Stale bool `json:"stale"` }

func EffectivePrice(o Offer, now time.Time)*int64 {
 if o.Price==nil || o.PriceKind!="exact" || o.PriceCheckedAt.IsZero() || now.Sub(o.PriceCheckedAt)>30*24*time.Hour || (o.ValidUntil!=nil&&!now.Before(*o.ValidUntil)){return nil};return o.Price
}
func Search(courses []Course, f Filter, now time.Time) []Result {
 out:=[]Result{}
 for _,c:=range courses{
 if c.Status!="published" || (f.Language!=""&&c.Language!=f.Language)||(f.Direction!=""&&c.Direction!=f.Direction){continue}
 if f.Experience!="" {
   if f.Experience=="experienced" { ok:=false;for _,a:=range c.Audience{if a!="none"{ok=true}};if !ok{continue}
   } else if !slices.Contains(c.Audience,f.Experience){continue}
 }
 if f.Goal!=""&&!slices.Contains(c.Goals,f.Goal){continue}
 candidates:=[]Result{}
 for _,o:=range c.Offers{
 if !f.IncludeClosed&&o.Enrollment!="open"&&o.Enrollment!="continuous"{continue}
 price:=EffectivePrice(o,now)
 if f.Budget=="free"&&(!o.Free||price==nil||*price!=0){continue}
 if f.Budget=="paid"&&o.Free&&!f.IncludeFree{continue}
 if f.Min!=nil||f.Max!=nil {if !(f.IncludeFree&&o.Free&&price!=nil&&*price==0){if price==nil||(f.Min!=nil&&*price<*f.Min)||(f.Max!=nil&&*price>*f.Max){continue}}}
 if f.Hours!=nil&&(o.Hours==nil||*o.Hours>*f.Hours){continue}
 if f.Support=="review"&&!o.Review||f.Support=="mentor"&&!o.Mentor||f.Support=="self"&&(o.Review||o.Mentor){continue}
 if f.Schedule!=""&&o.Schedule!=f.Schedule{continue}
 reasons:=[]string{}
 if f.Goal!=""{reasons=append(reasons,"Соответствует вашей цели")}
 if f.Experience!=""{reasons=append(reasons,"Подходит вашему опыту")}
 if f.Min!=nil||f.Max!=nil{reasons=append(reasons,"В пределах бюджета")}
 if f.Support=="review"{reasons=append(reasons,"Проверка кода человеком")}
 if f.Hours!=nil{reasons=append(reasons,"Подходит по нагрузке")}
 if len(reasons)>3{reasons=reasons[:3]}
 candidates=append(candidates,Result{Course:c,Offer:o,Price:price,Reasons:reasons,Stale:now.Sub(c.CheckedAt)>90*24*time.Hour})
 }
 sort.SliceStable(candidates,func(i,j int)bool{a,b:=candidates[i],candidates[j];if a.Price==nil&&b.Price!=nil{return false};if a.Price!=nil&&b.Price==nil{return true};if a.Price!=nil&&b.Price!=nil&&*a.Price!=*b.Price{return *a.Price<*b.Price};return a.Offer.ID<b.Offer.ID})
 if len(candidates)>0{out=append(out,candidates[0])}
 }
 sort.SliceStable(out,func(i,j int)bool{
 a,b:=out[i],out[j]
 if f.Sort=="price_asc"||f.Sort=="price_desc"{
 if a.Price==nil&&b.Price!=nil{return false};if a.Price!=nil&&b.Price==nil{return true}
 if a.Price!=nil&&b.Price!=nil&&*a.Price!=*b.Price{if f.Sort=="price_desc"{return *a.Price>*b.Price};return *a.Price<*b.Price}
 }else if f.Sort=="duration"{
 if a.Offer.Weeks==nil&&b.Offer.Weeks!=nil{return false};if a.Offer.Weeks!=nil&&b.Offer.Weeks==nil{return true}
 if a.Offer.Weeks!=nil&&b.Offer.Weeks!=nil&&*a.Offer.Weeks!=*b.Offer.Weeks{return *a.Offer.Weeks<*b.Offer.Weeks}
 }else{if a.Stale!=b.Stale{return !a.Stale};if !a.Course.CheckedAt.Equal(b.Course.CheckedAt){return a.Course.CheckedAt.After(b.Course.CheckedAt)}}
 return a.Course.ID<b.Course.ID
 });return out
}

