package command

import domainknowledge "myai/core/domain/knowledge"

type Retrieve struct {
	Query  domainknowledge.RetrievalQuery
	Strict bool
}
