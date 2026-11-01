`common-passwords.txt` is the 10,000-entry list from OWASP Passfault:
https://github.com/OWASP/passfault/blob/master/wordlists/wordlists/10k-worst-passwords.txt
The archived project distributes it under Apache-2.0; see PASSFAULT-LICENSE.txt.
The list is embedded at build time and never downloaded at runtime.

`long-common-passwords.txt` is the subset of entries of at least 15 characters
from Wikimedia CommonPasswords at commit
`b0afa0435ccc1d524664425d6c6ebd3bda5cdd65`:
https://github.com/wikimedia/mediawiki-libs-CommonPasswords/blob/b0afa0435ccc1d524664425d6c6ebd3bda5cdd65/data/common.php
Wikimedia's MIT license is in WIKIMEDIA-LICENSE.txt. Its upstream source is
SecLists, whose MIT license is in SECLISTS-LICENSE.txt. Only this relevant subset
is bundled; the full list is not required for passwords shorter than 15 characters.
