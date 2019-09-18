// Copyright 2018 flannel authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kube

import (
	"errors"
	"regexp"
	"strings"
)

type annotations struct {
	SubnetKubeManaged        string
	BackendData              string
	BackendType              string
	BackendPublicIP          string
	BackendPublicIPOverwrite string
}

// newAnnotations ...
// 验证 prefix 是否符合指定格式, 然后构建 annotation 对象.
func newAnnotations(prefix string) (annotations, error) {
	slashCnt := strings.Count(prefix, "/")
	if slashCnt > 1 {
		return annotations{}, errors.New("subnet/kube: prefix can contain at most single slash")
	}
	if slashCnt == 0 {
		prefix += "/"
	}
	if !strings.HasSuffix(prefix, "/") && !strings.HasSuffix(prefix, "-") {
		prefix += "-"
	}

	// matches is a regexp matching the format used by the kubernetes
	// for annotations. Following rules apply:
	// matches 是与kuber中使用的注解格式匹配的正则, 需要遵循如下规则:
	//	- must start with FQDN - must contain at most one slash "/"
	//	- must contain only lowercase letters, nubers, underscores,
	//	  hyphens, dots and slash
	// 1. 最多包含一个斜线
	// 2. 只能包含小写字母, 数字, 下划线, 中横线, 点号和斜线
	// FQDN: Fully Qualified Domain Name 完整域名
	matches, err := regexp.MatchString(`(?:[a-z0-9_-]+\.)+[a-z0-9_-]+/(?:[a-z0-9_-]+-)?$`, prefix)
	if err != nil {
		panic(err)
	}
	if !matches {
		return annotations{}, errors.New("subnet/kube: prefix must be in a format: fqdn/[0-9a-z-_]*")
	}

	a := annotations{
		SubnetKubeManaged:        prefix + "kube-subnet-manager",
		BackendData:              prefix + "backend-data",
		BackendType:              prefix + "backend-type",
		BackendPublicIP:          prefix + "public-ip",
		BackendPublicIPOverwrite: prefix + "public-ip-overwrite",
	}

	return a, nil
}
