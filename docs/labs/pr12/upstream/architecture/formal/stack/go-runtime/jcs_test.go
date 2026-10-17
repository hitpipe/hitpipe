package formalapi

import (
 "bytes"
 "crypto/sha256"
 "testing"
 "github.com/gowebpki/jcs"
)
func TestJCSRFC8785NumbersAndOrdering(t *testing.T) {
 vectors:=[]struct{input,want string}{
  {`{"b":2,"a":1}`,`{"a":1,"b":2}`},
  {`{"numbers":[333333333.33333329,1E30,4.50,2e-3,0.000000000000000000000000001]}`,`{"numbers":[333333333.3333333,1e+30,4.5,0.002,1e-27]}`},
  {`{"z":-0.0,"max":9007199254740991}`,`{"max":9007199254740991,"z":0}`},
  {`{"€":"Euro Sign","\r":"Carriage Return","דּ":"Hebrew","1":"One","😀":"Emoji","\u0080":"Control","ö":"Latin"}`,"{\"\\r\":\"Carriage Return\",\"1\":\"One\",\"\u0080\":\"Control\",\"ö\":\"Latin\",\"€\":\"Euro Sign\",\"😀\":\"Emoji\",\"דּ\":\"Hebrew\"}"},
  {`{"s":"<>&\u2028\u2029/"}`,"{\"s\":\"<>&\u2028\u2029/\"}"},
 }
 for _,v:=range vectors {got,err:=jcs.Transform([]byte(v.input));if err!=nil||string(got)!=v.want{t.Fatalf("in=%s got=%q want=%q err=%v",v.input,got,v.want,err)}}
}
func TestJCSRejectsAmbiguousOrInvalidJSON(t *testing.T){
 inputs:=[][]byte{[]byte(`{"a":1,"a":2}`),[]byte(`{"a":1,"\u0061":2}`),[]byte(`"\ud800"`),[]byte(`"\udc00"`),[]byte(`{"x":1e99999}`),[]byte(`{"x":NaN}`),[]byte(`{} {}`),[]byte(`{"x":01}`),{'"',0xff,'"'}}
 for _,in:=range inputs {if out,err:=jcs.Transform(in);err==nil{t.Fatalf("invalid accepted %q -> %q",in,out)}}
}
func TestJCSKeepsNullFalseZeroAbsentAndUnicodeDistinct(t *testing.T){
 inputs:=[]string{`{}`,`{"x":null}`,`{"x":false}`,`{"x":0}`,`{"x":"é"}`,`{"x":"e\u0301"}`};seen:=map[[32]byte]bool{}
 for _,in:=range inputs{out,err:=jcs.Transform([]byte(in));if err!=nil{t.Fatal(err)};h:=sha256.Sum256(out);if seen[h]{t.Fatalf("distinct data collapsed %s -> %s",in,out)};seen[h]=true}
 a,_:=jcs.Transform([]byte(`{"b":2,"a":1}`));b,_:=jcs.Transform([]byte(` { "a" : 1 , "b" : 2 } `));if !bytes.Equal(a,b){t.Fatal("equivalent formatting not canonical")}
}
