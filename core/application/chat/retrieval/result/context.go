package result

import searchresult "myai/core/application/knowledge/search/result"

type Context struct {
	Triggered bool
	Query     string
	Prompt    string
	Search    searchresult.Search
	Error     string
}
