import 'i18n_tables.dart';

/// The interface's languages, in the order the settings list them.
const languages = ['zh', 'zh-Hant', 'en', 'ja'];

/// Each language as the settings show it, in itself.
const languageNames = {'zh': '简体中文', 'zh-Hant': '繁體中文', 'en': 'English', 'ja': '日本語'};

/// The interface's language; `AppState.setLanguage` changes it.
String language = 'zh';

/// The pair's string in the interface's language. Other languages are tables
/// keyed by the Chinese, as on the desktop: a missing Japanese falls back to
/// English and a missing Traditional to Simplified. `{0}`, `{1}` take `args`.
///
/// Keep `zh` a literal so the tables can be checked against it, and call this
/// while building, never for a top-level value, so a word follows a switch.
String tr(String zh, String en, [List<Object?> args = const []]) {
  var s = switch (language) {
    'en' => en,
    'ja' => jaTable[zh] ?? en,
    'zh-Hant' => hantTable[zh] ?? zh,
    _ => zh,
  };
  for (final (i, a) in args.indexed) {
    s = s.replaceAll('{$i}', '$a');
  }
  return s;
}

/// A skin's name; `skinDefs` holds the Chinese.
String skinName(String id) => switch (id) {
      'paper' => tr('纸墨', 'Paper'),
      'broadsheet' => tr('报刊', 'Broadsheet'),
      'phosphor' => tr('荧光屏', 'Phosphor'),
      'neon' => tr('霓虹', 'Neon'),
      'blueprint' => tr('蓝图', 'Blueprint'),
      'sakura' => tr('樱', 'Sakura'),
      _ => id,
    };
