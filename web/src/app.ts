import htmx from 'htmx.org';
import type { Root } from 'react-dom/client';

htmx.config.allowEval = false;
htmx.config.allowScriptTags = false;
htmx.config.historyCacheSize = 0;
htmx.config.includeIndicatorStyles = false;
htmx.config.selfRequestsOnly = true;
const mounted = new Map<HTMLElement, Root | null>();
const registry = { dashboard: () => import('./dashboard') };
function mount(scope: ParentNode) {
 const elements = [...scope.querySelectorAll<HTMLElement>('[data-island]')];
 if (scope instanceof HTMLElement && scope.matches('[data-island]')) elements.unshift(scope);
 for (const el of elements) {
  if (mounted.has(el)) continue;
  mounted.set(el,null);
  const name = el.dataset.island;
  if (name !== 'dashboard') { fail(el); continue; }
  void registry[name]().then(({mount})=>{
   if (!el.isConnected || !mounted.has(el)) return;
   mounted.set(el,mount(el,JSON.parse(el.dataset.props ?? '{}')));
  }).catch(()=>{if(el.isConnected && mounted.has(el)) fail(el);});
 }
}
function fail(el:HTMLElement){el.replaceChildren(Object.assign(document.createElement('p'),{textContent:'This overview could not load. Reload the page or use Requests.'}));el.setAttribute('role','alert');}
function cleanup(scope:Element){for(const[el,root]of mounted){if(el===scope||scope.contains(el)){root?.unmount();mounted.delete(el)}}}
document.addEventListener('htmx:beforeCleanupElement',event=>cleanup((event as CustomEvent<{elt:Element}>).detail.elt));
document.addEventListener('htmx:load',event=>mount((event as CustomEvent<{elt:HTMLElement}>).detail.elt));
for(const event of ['htmx:responseError','htmx:sendError']) document.addEventListener(event,()=>{const el=document.getElementById('network-error');if(el)el.hidden=false});
try{document.documentElement.classList.toggle('dark',localStorage.getItem('theme')==='dark')}catch{/* Storage can be disabled. */}
document.getElementById('theme-toggle')?.addEventListener('click',()=>{const dark=document.documentElement.classList.toggle('dark');try{localStorage.setItem('theme',dark?'dark':'light')}catch{/* Theme remains usable without persistence. */}});
mount(document);
