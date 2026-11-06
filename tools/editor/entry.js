// The code editor of the file manager (CodeMirror 6). It is built with code splitting: this entry holds the editor itself,
// and the grammar of each language is a separate file that is fetched only when a file of that kind is opened.
import { EditorState, Compartment } from '@codemirror/state';
import {
  EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter, drawSelection,
  highlightSpecialChars, rectangularSelection, dropCursor
} from '@codemirror/view';
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import { searchKeymap, highlightSelectionMatches } from '@codemirror/search';
import {
  syntaxHighlighting, defaultHighlightStyle, indentOnInput, bracketMatching, foldGutter, foldKeymap, StreamLanguage
} from '@codemirror/language';
import { closeBrackets, closeBracketsKeymap } from '@codemirror/autocomplete';
import { oneDark } from '@codemirror/theme-one-dark';

const legacy = (path, name) => import(path).then(m => StreamLanguage.define(m[name]));
const loaders = {
  php: () => import('@codemirror/lang-php').then(m => m.php()),
  html: () => import('@codemirror/lang-html').then(m => m.html()),
  css: () => import('@codemirror/lang-css').then(m => m.css()),
  js: () => import('@codemirror/lang-javascript').then(m => m.javascript()),
  jsx: () => import('@codemirror/lang-javascript').then(m => m.javascript({ jsx: true })),
  ts: () => import('@codemirror/lang-javascript').then(m => m.javascript({ typescript: true })),
  tsx: () => import('@codemirror/lang-javascript').then(m => m.javascript({ typescript: true, jsx: true })),
  json: () => import('@codemirror/lang-json').then(m => m.json()),
  markdown: () => import('@codemirror/lang-markdown').then(m => m.markdown()),
  sql: () => import('@codemirror/lang-sql').then(m => m.sql()),
  xml: () => import('@codemirror/lang-xml').then(m => m.xml()),
  yaml: () => import('@codemirror/lang-yaml').then(m => m.yaml()),
  shell: () => import('@codemirror/legacy-modes/mode/shell').then(m => StreamLanguage.define(m.shell)),
  nginx: () => import('@codemirror/legacy-modes/mode/nginx').then(m => StreamLanguage.define(m.nginx)),
  properties: () => import('@codemirror/legacy-modes/mode/properties').then(m => StreamLanguage.define(m.properties))
};
void legacy;

// Which language a file is, from its name. Anything not listed is plain text, which needs nothing to load.
export function languageName(name) {
  const n = (name || '').toLowerCase();
  const ext = n.indexOf('.') >= 0 ? n.slice(n.lastIndexOf('.') + 1) : '';
  if (n === '.htaccess' || n === 'nginx.conf' || (/\.conf$/.test(n) && /nginx|vhost/.test(n))) return 'nginx';
  if (n === '.env' || /^\.env\./.test(n) || ['ini', 'properties', 'cnf'].includes(ext)) return 'properties';
  return {
    php: 'php', phtml: 'php', inc: 'php', html: 'html', htm: 'html', twig: 'html', tpl: 'html', css: 'css', scss: 'css', less: 'css',
    js: 'js', mjs: 'js', cjs: 'js', jsx: 'jsx', ts: 'ts', tsx: 'tsx', json: 'json', map: 'json', md: 'markdown', markdown: 'markdown',
    sql: 'sql', xml: 'xml', svg: 'xml', xsl: 'xml', yml: 'yaml', yaml: 'yaml', sh: 'shell', bash: 'shell', zsh: 'shell'
  }[ext] || '';
}

const isDark = () => document.documentElement.classList.contains('dark');
const themeFor = dark => dark ? oneDark : [syntaxHighlighting(defaultHighlightStyle, { fallback: true }), EditorView.theme({ '&': { backgroundColor: '#ffffff' } })];

// create(parent, {doc, name, nonce, onChange}) shows an editor at once, as plain text, and adds the language when its
// file has arrived. It returns the view.
export function create(parent, opts) {
  const theme = new Compartment(), lang = new Compartment();
  const view = new EditorView({
    parent,
    state: EditorState.create({
      doc: opts.doc || '',
      extensions: [
        EditorView.cspNonce.of(opts.nonce || ''), // the page's policy allows only style elements that carry this nonce
        lineNumbers(), highlightActiveLineGutter(), highlightSpecialChars(), history(), foldGutter(), drawSelection(), dropCursor(),
        EditorState.allowMultipleSelections.of(true), indentOnInput(), bracketMatching(), closeBrackets(),
        rectangularSelection(), highlightActiveLine(), highlightSelectionMatches(),
        keymap.of([...closeBracketsKeymap, ...defaultKeymap, ...searchKeymap, ...historyKeymap, ...foldKeymap, indentWithTab]),
        lang.of([]),
        theme.of(themeFor(isDark())),
        EditorView.theme({
          '&': { height: '100%', fontSize: '14px' },
          '.cm-scroller': { fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace', lineHeight: '1.5' }
        }),
        EditorView.updateListener.of(u => { if (u.docChanged && opts.onChange) opts.onChange(); })
      ]
    })
  });
  const which = languageName(opts.name);
  if (loaders[which]) loaders[which]().then(ext => view.dispatch({ effects: lang.reconfigure(ext) })).catch(() => {});
  new MutationObserver(() => view.dispatch({ effects: theme.reconfigure(themeFor(isDark())) }))
    .observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
  return view;
}
