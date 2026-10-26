package internal

import "github.com/open-enu/kernel/flags"

const pageSize = "demo.page_size"

var Flags = []flags.Definition{
	{Identifier: "demo.archive", Name: "Archive", Default: false},
	{pageSize, "Page size", "", 20, "int"},
}
