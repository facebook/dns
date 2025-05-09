/*
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package dnsdata

import (
	"bytes"
	"io"
	"testing"
)

var in = []byte(`+foo.test,::1
Zfoo.test,ns.test,dns.test,,1800,900,604800,3600,300
Zbar.test,ns.test,dns.test,66223344,1800,900,604800,3600,300
%\000\001,0.0.0.0/0,m1
%ab,2a00:1fa0:42d8::/64,m1
%a\001,192.168.1.0/24,m1
%on,10.0.0.0/8,m1
# ignored line
+bar.test,127.0.0.1
`)

var targ = []byte(`+foo.test,::1
Zfoo.test,ns.test,dns.test,123456,1800,900,604800,3600,300,,
Zbar.test,ns.test,dns.test,66223344,1800,900,604800,3600,300,,
+bar.test,127.0.0.1
!\155\061,::
!\155\061,0.0.0.0,0,\000\001
!\155\061,10.0.0.0,8,\157\156
!\155\061,11.0.0.0,0,\000\001
!\155\061,192.168.1.0,24,\141\001
!\155\061,192.168.2.0,0,\000\001
!\155\061,::1:0:0:0
!\155\061,2a00:1fa0:42d8::,64,\141\142
!\155\061,2a00:1fa0:42d8:1::
`)

func TestPreprocess(t *testing.T) {
	codec := new(Codec)
	codec.Acc.Ranger.Enable()
	codec.Acc.NoPrefixSets = true
	codec.NoRnetOutput = true
	codec.Serial = 123456

	r := bytes.NewReader(in)
	outbuf := new(bytes.Buffer)

	if err := codec.Preprocess(r, outbuf); err != nil {
		t.Fatal(err)
	}
	out := outbuf.Bytes()
	if !bytes.Equal(out, targ) {
		t.Fatalf("encoded differs: \n---- got:\n%s\n---- expected:\n%s====", out, targ)
	}
}

func TestPreprocReader(t *testing.T) {
	codec := new(Codec)
	codec.Acc.Ranger.Enable()
	codec.Acc.NoPrefixSets = true
	codec.NoRnetOutput = true
	codec.Serial = 123456

	r := bytes.NewReader(in)
	outbuf := new(bytes.Buffer)

	preprocReader := NewPreprocReader(r, codec)

	_, err := io.Copy(outbuf, preprocReader)
	if err != nil {
		t.Fatal(err)
	}
	out := outbuf.Bytes()
	if !bytes.Equal(out, targ) {
		t.Fatalf("encoded differs: \n---- got:\n%s\n---- expected:\n%s====", out, targ)
	}
}
