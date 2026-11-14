import 'package:bowel/ui/atoms.dart';
import 'package:bowel/ui/skin.dart';
import 'package:flutter/material.dart' show DefaultMaterialLocalizations, Material, MaterialType, TextField;
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

// What the field asks the platform keyboard for when it gets focus. On
// Android, enableSuggestions: false becomes a visible-password input type,
// and some keyboards answer that with their secure mode.
Future<Map<String, dynamic>> _config(WidgetTester tester, TextInputType? type, {int? maxLines = 1}) async {
  await tester.pumpWidget(Localizations(
    locale: const Locale('en'),
    delegates: const [DefaultMaterialLocalizations.delegate, DefaultWidgetsLocalizations.delegate],
    child: MediaQuery(
      data: const MediaQueryData(size: Size(400, 800)),
      child: Skin(
        look: Look.of('paper', false),
        child: Overlay(initialEntries: [
          OverlayEntry(builder: (_) => Material(type: MaterialType.transparency, child: BareField(hint: 'hint', keyboardType: type, maxLines: maxLines))),
        ]),
      ),
    ),
  ));
  tester.testTextInput.log.clear();
  await tester.showKeyboard(find.byType(TextField));
  final call = tester.testTextInput.log.lastWhere((c) => c.method == 'TextInput.setClient');
  return ((call.arguments as List)[1] as Map).cast<String, dynamic>();
}

void main() {
  testWidgets('a plain field keeps suggestions on', (tester) async {
    expect((await _config(tester, null))['enableSuggestions'], isTrue);
  });

  testWidgets('a link field keeps suggestions on and autocorrect off', (tester) async {
    final c = await _config(tester, TextInputType.url);
    expect(c['enableSuggestions'], isTrue);
    expect(c['autocorrect'], isFalse);
  });

  testWidgets('the multiline subscription field keeps suggestions on', (tester) async {
    expect((await _config(tester, TextInputType.multiline, maxLines: null))['enableSuggestions'], isTrue);
  });

  testWidgets('a secret field turns suggestions off', (tester) async {
    expect((await _config(tester, TextInputType.visiblePassword))['enableSuggestions'], isFalse);
  });
}
