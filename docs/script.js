(function() {
	// ---------- Theme ----------
	const themeDropdown = document.querySelector('.theme-dropdown');
	const themeTrigger = document.querySelector('.theme-trigger');
	const themeOptions = document.querySelectorAll('.theme-option');

	// ---------- Lang ----------
	const langDropdown = document.querySelector('.lang-dropdown');
	const langTrigger = document.querySelector('.lang-trigger');
	const langOptions = document.querySelectorAll('.lang-option');

	const LANG_PREFIX = 'pg_atropos_lang';
	const THEME_PREFIX = 'pg_atropos_theme';
	const SUPPORTED_LANGS = ['cs','en','es','fr','de','it','pt','el','uk','pl'];

	const translations = window.PG_TRANSLATIONS || {};

	// ---------- Storage helpers ----------
	function getStored(key, fallback) {
		try { return localStorage.getItem(key) || fallback; } catch(e) { return fallback; }
	}

	function setStored(key, val) {
		try { localStorage.setItem(key, val); } catch(e) {}
	}

	// ---------- Theme ----------
	function applyTheme(theme) {
		if (theme === 'auto') {
			document.documentElement.removeAttribute('data-theme');
		} else {
			document.documentElement.setAttribute('data-theme', theme);
		}
		setStored(THEME_PREFIX, theme);
		themeOptions.forEach(function(opt) {
			opt.classList.toggle('active', opt.getAttribute('data-theme-value') === theme);
		});
		if (themeDropdown) themeDropdown.classList.remove('open');
	}

	// ---------- Language ----------
	function resolveLang(lang) {
		if (lang === 'auto') {
			var nav = navigator.languages || [navigator.language];
			for (var i = 0; i < nav.length; i++) {
				var s = nav[i].split('-')[0].toLowerCase();
				if (translations[s]) return s;
			}
			return 'en';
		}
		return lang;
	}

	function applyLang(selectedLang) {
		var target = resolveLang(selectedLang);
		document.documentElement.setAttribute('lang', target);
		setStored(LANG_PREFIX, selectedLang);

		document.querySelectorAll('[data-t]').forEach(function(el) {
			var key = el.getAttribute('data-t');
			if (translations[target] && translations[target][key]) {
				el.innerHTML = translations[target][key];
			}
		});

		langOptions.forEach(function(opt) {
			opt.classList.toggle('active', opt.getAttribute('data-lang-value') === selectedLang);
		});
		if (langDropdown) langDropdown.classList.remove('open');
	}

	// ---------- Copy buttons ----------
	function copyText(text, btn) {
		var done = function() {
			btn.classList.add('copied');
			var orig = btn.innerHTML;
			btn.innerHTML = '<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"/></svg>';
			setTimeout(function() {
				btn.classList.remove('copied');
				btn.innerHTML = orig;
			}, 2000);
		};
		if (navigator.clipboard) {
			navigator.clipboard.writeText(text).then(done)['catch'](function() { fallback(text, done); });
		} else {
			fallback(text, done);
		}
	}

	function fallback(text, done) {
		var ta = document.createElement('textarea');
		ta.value = text;
		ta.style.position = 'fixed';
		ta.style.opacity = '0';
		document.body.appendChild(ta);
		ta.select();
		try { document.execCommand('copy'); done(); } catch(e) {}
		document.body.removeChild(ta);
	}

	function setupCopyButtons() {
		document.querySelectorAll('.copy-mini').forEach(function(btn) {
			btn.addEventListener('click', function(e) {
				e.stopPropagation();
				var text = btn.getAttribute('data-copy');
				if (!text) return;
				copyText(text, btn);
			});
		});
	}

	// ---------- Dropdown handlers ----------
	function setupDropdown(trigger, dropdown) {
		if (!trigger || !dropdown) return;
		trigger.addEventListener('click', function(e) {
			e.stopPropagation();
			var wasOpen = dropdown.classList.contains('open');
			document.querySelectorAll('.lang-dropdown, .theme-dropdown').forEach(function(d) { d.classList.remove('open'); });
			if (!wasOpen) dropdown.classList.add('open');
			trigger.setAttribute('aria-expanded', !wasOpen);
		});
	}

	// ---------- Init ----------
	applyTheme(getStored(THEME_PREFIX, 'auto'));
	applyLang(getStored(LANG_PREFIX, 'auto'));

	setupDropdown(themeTrigger, themeDropdown);
	setupDropdown(langTrigger, langDropdown);

	themeOptions.forEach(function(opt) {
		opt.addEventListener('click', function() {
			applyTheme(opt.getAttribute('data-theme-value'));
		});
	});

	langOptions.forEach(function(opt) {
		opt.addEventListener('click', function() {
			applyLang(opt.getAttribute('data-lang-value'));
		});
	});

	document.addEventListener('click', function() {
		if (themeDropdown) themeDropdown.classList.remove('open');
		if (langDropdown) langDropdown.classList.remove('open');
	});

	setupCopyButtons();

})();
