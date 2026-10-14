/*
Copyright 2026 The pdfcpu Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

const (
	usageLongPerm = `Manage user access permissions.

      perm ... user access permissions
    inFile ... input PDF file, use - to read from stdin
   outFile ... output PDF file, use - to write to stdout

   perm modes:
           none: 000000000000 (x000)
          print: 100000000100 (x804)
            all: 111100111100 (xF3C)

   or perm explicitly:
         'x' + max. 3 hex digits (max3Hex, eg. xF30)
         max. 12 binary digits (max12Bits, eg. 111100110000)

   using the permission bits:

      1:  -
      2:  -
      3:  Print (security handlers rev.2), draft print (security handlers >= rev.3)
      4:  Modify contents by operations other than controlled by bits 6, 9, 11.
      5:  Copy, extract text & graphics
      6:  Add or modify annotations, fill form fields, in conjunction with bit 4 create/mod form fields.
      7:  -
      8:  -
      9: Fill form fields (security handlers >= rev.3)
     10: Copy, extract text & graphics (security handlers >= rev.3) (unused since PDF 2.0)
     11: Assemble document (security handlers >= rev.3)
     12: Print (security handlers >= rev.3)

Pipeline examples:
   aws s3 cp s3://acme-legal/protected.pdf - \
      | pdfcpu permissions list -

   aws s3 cp s3://acme-legal/protected.pdf - \
      | pdfcpu permissions set --opw "$OPW" --perm print - - \
      | aws s3 cp - s3://acme-legal/printable.pdf`

	usageLongEncrypt = `Setup password protection based on user and owner password.

      mode ... algorithm (default=aes)
       key ... key length in bits (default=256)
      perm ... user access permissions
    inFile ... input PDF file, use - to read from stdin
   outFile ... output PDF file, use - to write to stdout

PDF 2.0 files have to be encrypted using aes/256.

Pipeline example:
   aws s3 cp s3://acme-hr/onboarding.pdf - \
      | pdfcpu encrypt --opw "$OPW" --upw "$UPW" - - \
      | aws s3 cp - s3://acme-hr/secure/onboarding.pdf`

	usageLongDecrypt = `Remove password protection and reset permissions.

    inFile ... input PDF file, use - to read from stdin
   outFile ... output PDF file, use - to write to stdout

Pipeline example:
   aws s3 cp s3://acme-hr/secure/onboarding.pdf - \
      | pdfcpu decrypt --upw "$UPW" - - \
      | aws s3 cp - s3://acme-hr/plain/onboarding.pdf`

	usageLongChangeUserPW = `Change the user password also known as the open doc password.

       opw ... owner password, required unless = ""
    inFile ... input PDF file, use - to read from stdin
    upwOld ... old user password
    upwNew ... new user password
   outFile ... output PDF file, use - to write to stdout

Pipeline example:
   aws s3 cp s3://acme-legal/client.pdf - \
      | pdfcpu changeupw --opw "$OPW" - "$OLD_UPW" "$NEW_UPW" - \
      | aws s3 cp - s3://acme-legal/client-rotated-upw.pdf`

	usageLongChangeOwnerPW = `Change the owner password also known as the set permissions password.

       upw ... user password, required unless = ""
    inFile ... input PDF file, use - to read from stdin
    opwOld ... old owner password (provide user password on initial changeopw)
    opwNew ... new owner password
   outFile ... output PDF file, use - to write to stdout

Pipeline example:
   aws s3 cp s3://acme-legal/client.pdf - \
      | pdfcpu changeopw --upw "$UPW" - "$OLD_OPW" "$NEW_OPW" - \
      | aws s3 cp - s3://acme-legal/client-rotated-opw.pdf`
)
