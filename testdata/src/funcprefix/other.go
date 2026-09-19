package funcprefix

type Repo struct{}

func (r *Repo) GetByID(id int) {} // want "func-prefix: matched"

func (Repo) isEmpty() bool { return true } // want "func-prefix: matched"

func (r *Repo) Save() {}

// Only function declarations have a name and a body to compare.
var GetDefault = func() {}

type Reader interface {
	GetName() string
	IsReady() bool
}

// Without a body there is nothing to ask about.
func GetCPUFeatures() uint64
