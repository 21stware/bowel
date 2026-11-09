import 'dart:io';

import 'package:bowel/app/i18n.dart';
import 'package:bowel/app/i18n_tables.dart';
import 'package:flutter_test/flutter_test.dart';

final _pair = RegExp(r"\btr\(\s*'((?:[^'\\]|\\.)*)'");
final _word = RegExp(r"'((?:[^'\\\n]|\\.)*[一-鿿](?:[^'\\\n]|\\.)*)'");
final _slot = RegExp(r'\{\d\}');

/// Chinese that is not words on screen: sample data, keys the code compares,
/// and the source of each translation.
const _notShown = {'lib/app/sample.dart', 'lib/app/i18n_tables.dart', 'lib/ui/skin_data.dart'};
const _keys = {'默认', 'AI 服务', '流媒体', '开发', '直连', '香港 03 IPLC', '洛杉矶 VPS', '新加坡 01', '东京 01', 'mihomo · 运行中', '未知', '实测', '按名称', '简体中文', '繁體中文', '日本語'};

Iterable<File> get _sources => Directory('lib').listSync(recursive: true).whereType<File>().where((f) => f.path.endsWith('.dart'));

Set<String> get _used => {
      for (final f in _sources)
        if (!f.path.endsWith('i18n_tables.dart'))
          for (final m in _pair.allMatches(f.readAsStringSync())) m.group(1)!,
    };

void main() {
  test('every tr has Japanese and Traditional, with the same slots', () {
    for (final zh in _used) {
      for (final (name, table) in [('ja', jaTable), ('zh-Hant', hantTable)]) {
        final s = table[zh];
        expect(s, isNotNull, reason: '$name lacks "$zh"');
        expect(_slot.allMatches(s!).map((m) => m[0]).toSet(), _slot.allMatches(zh).map((m) => m[0]).toSet(), reason: '$name "$zh"');
      }
    }
  });

  test('no Chinese on screen outside tr', () {
    for (final f in _sources) {
      final path = f.path.replaceAll(r'\', '/');
      if (_notShown.contains(path)) continue;
      for (final line in f.readAsLinesSync()) {
        if (line.trimLeft().startsWith('//')) continue;
        final rest = line.replaceAll(_pair, '');
        for (final m in _word.allMatches(rest)) {
          expect(_keys, contains(m[1]), reason: '$path: $line');
        }
      }
    }
  });

  test('tr picks the language and fills slots', () {
    addTearDown(() => language = 'zh');
    expect(tr('{0} 条规则', '{0} rules', [3]), '3 条规则');
    language = 'en';
    expect(tr('{0} 条规则', '{0} rules', [3]), '3 rules');
    language = 'ja';
    expect(tr('{0} 条规则', '{0} rules', [3]), '3 件のルール');
    expect(tr('表里没有的词', 'Not in the table'), 'Not in the table');
    language = 'zh-Hant';
    expect(tr('{0} 条规则', '{0} rules', [3]), '3 條規則');
    expect(tr('表里没有的词', 'Not in the table'), '表里没有的词');
  });
}
