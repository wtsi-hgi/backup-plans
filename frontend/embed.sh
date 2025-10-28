#!/bin/bash

(
	cd "src";
	head -n1 index.html;
	head -n4 index.html | tail -n+2 | tr -d '\n\t';
	echo -n "<style type=\"text/css\">";
	while read -r line; do
		if [ "${line:0:11}" = "@import url" ]; then
			cat "${line:13:-3}";
		else
			echo "$line";
		fi;
	done < style.css | tr -d '\n\t';
	echo -n "</style>";
	echo -n "<script type=\"module\">";
	jspacker -i "/$(grep "<script" index.html | sed -e 's/.*src="\([^"]*\)".*/\1/')" -n | if command -v terser > /dev/null; then terser -m  --module --compress pure_getters,passes=3 --ecma 2020 | tr -d '\n'; else tr -d '\n\t'; fi;
	echo -n "</script>";
	tail -n2 index.html | tr -d '\n	';
) > "index.html";

declare size="$(stat -c %s index.html)";

if command -v zopfli > /dev/null; then
	zopfli -m index.html;
	rm -f index.html;
else
	gzip -f -9 index.html;
fi;

declare time="$(stat -c %Y index.html.gz)"

cat > frontend.go <<HEREDOC
//go:build !dev
// +build !dev

package frontend

// File automatically generated with ./embed.sh

import (
	_ "embed"
	"time"

	"vimagination.zapto.org/httpembed"
)

//go:embed index.html.gz
var indexHTML []byte

const (
	uncompressedSize = $size
	lastModifiedTime = $time
)

var Index = httpembed.HandleBuffer("index.html", indexHTML, uncompressedSize, time.Unix(lastModifiedTime, 0)) //nolint:gochecknoglobals,lll
HEREDOC