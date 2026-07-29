package public

const Constant = 1

var Variable = "public"

type Exported struct{}

type Excluded struct{}

type Alias = Exported

func Function() {}

func (Exported) ValueMethod() {}

func (*Exported) PointerMethod() {}

type unexported struct{}

func unexportedFunction() {}
