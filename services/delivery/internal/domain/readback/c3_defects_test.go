package readback

import "testing"

func TestC3FalseConfirmationGuards(t *testing.T) {
	block := "<!-- hitpipe:handoff:start -->x<!-- hitpipe:handoff:end -->"
	baseExpected := Expected{Workspace:"Ops", List:"Delivery", Title:"Deploy", Assignee:"dima", HandoffBlock:block, Context:"ctx"}
	baseObserved := Observation{Retrieved:true, Workspace:"Ops", List:"Delivery", Title:"Deploy", Assignees:[]string{"dima"}, HandoffBlock:block, Context:"ctx"}
	cases := []struct{name string; mutate func(*Observation); want Outcome}{
		{"retrieval missing is unknown", func(o *Observation){o.Retrieved=false}, Unknown},
		{"duplicate start marker is incomplete", func(o *Observation){o.HandoffBlock=block+"<!-- hitpipe:handoff:start -->"}, Incomplete},
		{"title change is mismatch", func(o *Observation){o.Title="Deploy other"}, Mismatch},
		{"invalid utf8 is unknown", func(o *Observation){o.Context=string([]byte{0xff})}, Unknown},
		{"missing assignee is mismatch", func(o *Observation){o.Assignees=nil}, Mismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o:=baseObserved; tc.mutate(&o)
			if got:=Compare(baseExpected,o); got.Outcome != tc.want { t.Fatalf("got %#v want %s",got,tc.want) }
		})
	}
}
