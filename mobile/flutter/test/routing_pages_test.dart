import 'dart:convert';

import 'package:bowel/app/i18n.dart';
import 'package:bowel/app/state.dart';
import 'package:bowel/board/cards.dart';
import 'package:bowel/board/fly.dart';
import 'package:bowel/pages/frame.dart';
import 'package:bowel/pages/routing.dart';
import 'package:bowel/pages/sheet.dart';
import 'package:bowel/ui/atoms.dart';
import 'package:bowel/ui/skin.dart';
import 'package:flutter/material.dart' show DefaultMaterialLocalizations, Material, MaterialType;
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

Map _service(String id, String name, {bool builtin = false, bool mine = false, bool off = false, List<String> rules = const []}) => {
      'id': id,
      'name': name,
      'builtin': builtin,
      'mine': mine,
      'off': off,
      'changed': false,
      'rules': [
        for (final r in rules) {'text': 'DOMAIN-SUFFIX,$r', 'kind': 'suffix', 'kindName': '域名后缀', 'value': r, 'shown': r, 'builtin': builtin, 'off': false},
      ],
    };

const _dnsRules = ['dns.alidns.com', 'doh.360.cn', 'doh.pub', '1.12.12.12/32'];

List<Map> _preview(int n) => [
      for (final r in _dnsRules.take(n)) {'text': 'DOMAIN-SUFFIX,$r', 'kind': 'suffix', 'kindName': '域名后缀', 'value': r, 'shown': r},
    ];

/// A rule set as the bridge lists it: no rules of its own, and a `set`.
Map _ruleSet(String id, String name, {bool local = false, bool have = true, bool off = false, String error = '', String target = '', int count = 4, int skipped = 0}) => {
      ..._service(id, name, off: off),
      'set': {
        'where': local ? 'ChinaDNS.yaml' : 'rules.example.com',
        'local': local,
        'target': target,
        'have': have,
        'count': have ? count : 0,
        'skipped': skipped,
        'updatedAt': DateTime.now().subtract(const Duration(hours: 2)).millisecondsSinceEpoch,
        'error': error,
        'preview': have ? _preview(4) : <Map>[],
      },
    };

/// The rule sets of 直连; a test puts its own here.
List<Map> _sets = [];

Map _group(String id, String name, List<Map> services, {bool direct = false, bool reject = false, String mode = 'auto'}) => {
      'id': id,
      'name': name,
      'direct': direct,
      'reject': reject,
      'custom': false,
      'movable': !const {'default', 'direct', 'reject'}.contains(id),
      'chooses': !const {'default', 'direct', 'reject'}.contains(id),
      'lockable': !const {'default', 'direct', 'reject'}.contains(id),
      'locked': id == 'ai' && _aiLocked,
      'mode': mode,
      'regions': <String>[],
      'providers': <String>[],
      'order': <String>[],
      'fixed': '',
      'now': '',
      'services': services,
      'targets': direct ? <Map>[] : [{'name': 'gstatic', 'url': 'https://www.gstatic.com/generate_204'}],
      'sites': <Map>[],
    };

/// Whether AI 服务 goes direct by the user's choice.
bool _aiDirect = false;

/// Whether AI 服务 has the exit lock.
bool _aiLocked = false;

/// Whether the bridge offers rules for apps, as on Android from 10.
bool _appKind = false;

/// The rules of AI 服务's 我的规则.
List<Map> _myAiRules = [];

/// The installed apps the phone lists, and the package it hides from bowel.
List<Map> _installed = [];
const _twoApps = [
  {'id': 'com.openai.chatgpt', 'name': 'ChatGPT'},
  {'id': 'com.google.android.gm', 'name': 'Gmail'},
];
const _hidden = 'com.oplus.framework.network.drs';

Map _appRule(String id) => {'text': 'PROCESS-NAME,$id', 'kind': 'app', 'kindName': '应用', 'value': id, 'shown': id, 'builtin': false, 'off': false};

Map _routing() => {
      'groups': [
        _group('default', '默认', [_service('mine:default', '我的规则', mine: true)]),
        _group('ai', 'AI 服务', direct: _aiDirect, [
          {..._service('mine:ai', '我的规则', mine: true), 'rules': _myAiRules},
          _service('openai', 'OpenAI', builtin: true, rules: ['openai.com', 'chatgpt.com', 'oaistatic.com']),
          _service('work', '工作', rules: ['work.example.com']),
        ]),
        _group('solo', '单项', [_service('mine:solo', '我的规则', mine: true), _service('only', '唯一', rules: ['only.example.com'])]),
        _group('direct', '直连', [_service('mine:direct', '我的规则', mine: true), ..._sets], direct: true),
        _group('reject', '拦截', [_service('mine:reject', '我的规则', mine: true), _service('ads', '广告', builtin: true, off: true, rules: ['ads.example.com'])], direct: true, reject: true),
      ],
      'nodes': [
        {'name': 'a|东京 01', 'display': '东京 01', 'provider': 'P1', 'providerId': 'p1', 'country': 'JP', 'flag': 'jp', 'delay': 80},
        {'name': 'b|香港 02', 'display': '香港 02', 'provider': 'P2', 'providerId': 'p2', 'country': 'HK', 'flag': 'hk', 'delay': -1},
      ],
      'regions': [
        {'code': 'JP', 'name': '东京', 'count': 1},
        {'code': 'HK', 'name': '香港', 'count': 1},
      ],
      'providers': [
        {'id': 'p1', 'name': 'P1'},
        {'id': 'p2', 'name': 'P2'},
      ],
      'kinds': [
        {'id': 'suffix', 'name': '域名后缀'},
        {'id': 'domain', 'name': '域名'},
        {'id': 'keyword', 'name': '域名关键词'},
        {'id': 'ip', 'name': 'IP 段'},
        {'id': 'geo', 'name': 'GeoSite 域名库'},
        {'id': 'geoip', 'name': 'GeoIP 地址库'},
        if (_appKind) {'id': 'app', 'name': '应用'},
      ],
    };

/// Answers the way the bridge would, and writes down the route operations.
final _ops = <Map>[];
String? _refuseWith;

/// The rule-set calls, in order, and what the next ones are refused with.
final _setCalls = <(String, Map)>[];
String? _checkFails, _updateFails, _importFails;
bool _targetImported = false;

/// What the bridge says of a source: a whole config when the link says so,
/// one of its targets, a link already imported, or the four DNS rules.
Map _checked(Map args) {
  final link = args['input'] as String? ?? '', target = args['text'] as String? ?? '';
  final where = (args['path'] as String? ?? '').isNotEmpty ? 'ChinaDNS.yaml' : 'rules.example.com';
  if (link.contains('dup') || (_targetImported && target == 'Proxy')) {
    return {'count': 0, 'skipped': 0, 'where': '', 'name': '', 'rules': <Map>[], 'dup': {'group': 'direct', 'groupName': '直连', 'service': 'set1'}};
  }
  if (link.contains('whole') && target.isEmpty) {
    return {
      'count': 0,
      'skipped': 0,
      'where': where,
      'name': '',
      'rules': <Map>[],
      'left': 3,
      'targets': [
        {'name': 'Proxy', 'count': 2, 'skipped': 2},
        {'name': 'DIRECT', 'count': 3, 'skipped': 0},
      ],
    };
  }
  if (target.isNotEmpty) return {'count': 2, 'skipped': 2, 'where': where, 'name': target, 'target': target, 'rules': _preview(2)};
  return {'count': 4, 'skipped': 0, 'where': where, 'name': 'ChinaDNS', 'rules': _preview(4)};
}

void _fakeCore() {
  TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger.setMockMethodCallHandler(const MethodChannel('bowel/shell'), (call) async {
    return switch (call.method) {
      'pickFile' => '/cache/picked/1/ChinaDNS.yaml',
      'listApps' => _installed,
      'canSeeApp' => (call.arguments as Map)['id'] != _hidden,
      _ => null,
    };
  });
  TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger.setMockMethodCallHandler(const MethodChannel('bowel/core'), (call) async {
    final method = call.arguments['method'] as String;
    final args = jsonDecode(call.arguments['payload'] as String) as Map;
    switch (method) {
      case 'routing':
        return jsonEncode(_routing());
      case 'parseRule':
        if (args['kind'] == 'geo') return jsonEncode({'kindName': 'GeoSite 域名库', 'value': (args['text'] as String).toLowerCase()});
        final uri = Uri.tryParse(args['text'] as String);
        if (uri == null) return jsonEncode({'error': 'Invalid address'});
        return jsonEncode({'kindName': '域名后缀', 'value': uri.host.isNotEmpty ? uri.host : args['text']});
      case 'checkRuleSet':
        _setCalls.add((method, args));
        if (_checkFails != null) throw PlatformException(code: 'x', message: _checkFails);
        return jsonEncode(_checked(args));
      case 'importRuleSet':
        _setCalls.add((method, args));
        if (_importFails != null) throw PlatformException(code: 'x', message: _importFails);
        return jsonEncode({'id': 'set1'});
      case 'updateRuleSet':
        _setCalls.add((method, args));
        if (_updateFails != null) throw PlatformException(code: 'x', message: _updateFails);
        return 'null';
      case 'routeOp':
        if ((args['op'] == 'addRule' || args['op'] == 'editRule') && _refuseWith != null) throw PlatformException(code: 'x', message: _refuseWith);
        _ops.add(args);
        return 'null';
    }
    return 'null';
  });
}

Widget _host(AppState app, Widget page, {double width = 360, double height = 760, double keyboard = 0}) => Localizations(
      locale: const Locale('en'),
      delegates: const [DefaultMaterialLocalizations.delegate, DefaultWidgetsLocalizations.delegate],
      child: MediaQuery(
        data: MediaQueryData(size: Size(width, height), viewInsets: EdgeInsets.only(bottom: keyboard)),
        child: Skin(
          look: Look.of('paper', false),
          child: Frame(
            data: const FrameData(top: 0, barInset: 0, nativeBar: false, homeIndicator: 0, form: WindowForm.phone),
            child: Align(
              alignment: Alignment.topLeft,
              child: SizedBox(
                width: width,
                height: height,
                child: Navigator(
                  onGenerateRoute: (_) => PageRouteBuilder<void>(
                    pageBuilder: (_, _, _) => Material(
                      type: MaterialType.transparency,
                      child: AnimatedBuilder(
                        animation: app,
                        builder: (context, _) => Stack(fit: StackFit.expand, children: [page, SheetHost(app: app)]),
                      ),
                    ),
                  ),
                ),
              ),
            ),
          ),
        ),
      ),
    );

Widget _card(AppState app) => FlyHost(
      child: CellMark(
        child: CellScope(cell: 'routes', shown: true, child: AnimatedBuilder(animation: app, builder: (context, _) => cardView(context, app, 'routes', 'expanded', 'wide'))),
      ),
    );

AppState _app() => AppState()..routing = _routing();

Future<void> _settle(WidgetTester tester) async {
  await tester.pump();
  await tester.pump(const Duration(milliseconds: 500));
  await tester.pump(const Duration(milliseconds: 500));
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  setUp(() {
    _ops.clear();
    _refuseWith = null;
    _setCalls.clear();
    _checkFails = _updateFails = _importFails = null;
    _targetImported = false;
    _sets = [];
    _aiDirect = false;
    _aiLocked = false;
    _appKind = false;
    _myAiRules = [];
    _installed = [..._twoApps];
    language = 'zh';
    _fakeCore();
  });

  tearDown(() => language = 'zh');

  for (final lang in ['zh', 'en', 'ja', 'zh-Hant']) {
    testWidgets('$lang: the pages fit a narrow phone', (tester) async {
      language = lang;
      tester.view.physicalSize = const Size(360, 760);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.reset);
      final app = _app();
      await tester.pumpWidget(_host(app, RoutesPage(app: app)));
      await _settle(tester);
      expect(tester.takeException(), isNull);
      for (final id in ['default', 'ai', 'direct', 'reject']) {
        await tester.pumpWidget(const SizedBox());
        await tester.pumpWidget(_host(app, GroupPage(app: app, id: id)));
        await _settle(tester);
        expect(tester.takeException(), isNull, reason: id);
      }
    });

    testWidgets('$lang: the route card footer holds the strategy on a narrow phone', (tester) async {
      language = lang;
      tester.view.physicalSize = const Size(360, 760);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.reset);
      final app = AppState()
        ..x = 'routes'
        ..sub = 'ai';
      await tester.pumpWidget(_host(app, _card(app)));
      await tester.pump(const Duration(seconds: 1));
      expect(tester.takeException(), isNull);
    });
  }

  testWidgets('the route card changes its strategy from a sheet', (tester) async {
    final app = AppState()
      ..x = 'routes'
      ..sub = 'ai';
    await tester.pumpWidget(_host(app, _card(app)));
    await tester.pump(const Duration(seconds: 1));
    await tester.tap(find.text('自动'));
    await _settle(tester);
    await tester.tap(find.text('顺序备用'));
    await _settle(tester);
    expect(app.sheet, isNull);
    expect(app.route('ai').policy, 'order');
    expect(find.text('备用'), findsOneWidget);
  });

  testWidgets('a built-in service takes rules, and a kind is left to be worked out', (tester) async {
    final app = _app();
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'ai', service: 'openai')));
    await _settle(tester);
    await tester.tap(find.text('添加规则').first);
    await _settle(tester);
    await tester.enterText(find.byType(EditableText), 'new.example.com');
    await _settle(tester);
    expect(find.text('添加后：new.example.com · 域名后缀'), findsOneWidget);
    await tester.tap(find.text('添加').last);
    await _settle(tester);
    expect(_ops.single['op'], 'addRule');
    expect(_ops.single['kind'], '');
    expect(_ops.single['service'], 'openai');
    expect(app.sheet, isNull);
  });

  testWidgets('a GeoSite list is a kind to pick, which says what to type', (tester) async {
    final app = _app();
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'ai', service: 'work')));
    await _settle(tester);
    await tester.tap(find.text('添加规则').last);
    await _settle(tester);
    // Past 自动识别 and the domain kinds: the row of kinds scrolls to it.
    final chips = find.ancestor(of: find.text('自动识别'), matching: find.byType(ListView));
    await tester.dragUntilVisible(find.text('GeoSite 域名库'), chips, const Offset(-120, 0));
    await _settle(tester);
    await tester.tap(find.text('GeoSite 域名库'));
    await _settle(tester);
    expect(find.text('写 GeoSite 列表的名字，列表里的域名都走这个路由组。'), findsOneWidget);
    expect(find.text('youtube、netflix 或 google@cn'), findsOneWidget, reason: 'the field says what a list is called');
    await tester.enterText(find.byType(EditableText), 'YouTube');
    await _settle(tester);
    expect(find.text('添加后：youtube · GeoSite 域名库'), findsOneWidget);
    await tester.tap(find.text('添加').last);
    await _settle(tester);
    expect([_ops.single['op'], _ops.single['kind'], _ops.single['text']], ['addRule', 'geo', 'YouTube']);
  });

  testWidgets('a rule whose list the data has not says it is not in use, and still opens', (tester) async {
    _myAiRules = [
      {'text': 'GEOSITE,nosuch', 'kind': 'geo', 'kindName': 'GeoSite 域名库', 'value': 'nosuch', 'shown': 'nosuch', 'builtin': false, 'off': false, 'unlisted': true},
      {'text': 'GEOSITE,youtube', 'kind': 'geo', 'kindName': 'GeoSite 域名库', 'value': 'youtube', 'shown': 'youtube', 'builtin': false, 'off': false},
    ];
    final app = _app();
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'ai', service: 'mine:ai')));
    await _settle(tester);
    expect(find.text('GeoSite 域名库 · 数据里没有这个列表，未生效'), findsOneWidget, reason: 'only the one the data lacks');
    expect(find.text('GeoSite 域名库'), findsOneWidget);
    await tester.tap(find.text('nosuch'));
    await _settle(tester);
    expect(find.text('编辑规则'), findsOneWidget);
  });

  testWidgets('a conflict stays in the sheet and offers to move the rule', (tester) async {
    _refuseWith = 'conflict:OpenAI';
    final app = _app();
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'ai', service: 'work')));
    await _settle(tester);
    await tester.tap(find.text('添加规则').last);
    await _settle(tester);
    await tester.enterText(find.byType(EditableText), 'openai.com');
    await _settle(tester);
    await tester.tap(find.text('添加').last);
    await _settle(tester);
    expect(app.sheet, isNotNull);
    expect(find.text('openai.com 已经在 OpenAI 里，一条规则只能属于一个服务。'), findsOneWidget);
    await tester.tap(find.text('移过来'));
    await _settle(tester);
    expect(_ops.single['op'], 'moveRule');
    expect(app.sheet, isNull);
  });

  testWidgets('a rule of the user\'s opens in the sheet to be edited, and keeps its group', (tester) async {
    final app = _app();
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'ai', service: 'work')));
    await _settle(tester);
    await tester.tap(find.text('work.example.com'));
    await _settle(tester);
    expect(find.text('编辑规则'), findsOneWidget);
    expect(tester.widget<EditableText>(find.byType(EditableText)).controller.text, 'work.example.com');
    await tester.enterText(find.byType(EditableText), 'chat.example.com');
    await _settle(tester);
    expect(find.text('保存后：chat.example.com · 域名后缀'), findsOneWidget);
    await tester.tap(find.text('保存'));
    await _settle(tester);
    final op = _ops.single;
    expect(op['op'], 'editRule');
    expect(op['old'], {'kind': 'suffix', 'value': 'work.example.com', 'route': 'ai', 'service': 'work'});
    expect([op['kind'], op['text'], op['route']], ['suffix', 'chat.example.com', 'ai']);
    expect(app.sheet, isNull);
  });

  testWidgets('a built-in rule does not open to be edited', (tester) async {
    final app = _app();
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'ai', service: 'openai')));
    await _settle(tester);
    await tester.tap(find.text('openai.com'));
    await _settle(tester);
    expect(app.sheet, isNull);
  });

  testWidgets('the edit sheet picks another group in place, and a refusal offers no move', (tester) async {
    _refuseWith = 'conflict:OpenAI';
    final app = _app();
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'ai', service: 'work')));
    await _settle(tester);
    await tester.tap(find.text('work.example.com'));
    await _settle(tester);
    await tester.tap(find.text('走'));
    await _settle(tester);
    await tester.tap(find.text('直连').last);
    await _settle(tester);
    expect(find.text('编辑规则'), findsOneWidget);
    expect(tester.widget<EditableText>(find.byType(EditableText)).controller.text, 'work.example.com');
    await tester.tap(find.text('保存'));
    await _settle(tester);
    expect(app.sheet, isNotNull);
    expect(find.text('work.example.com 已经在 OpenAI 里，一条规则只能属于一个服务。'), findsOneWidget);
    expect(find.text('移过来'), findsNothing);
    expect(_ops, isEmpty);
    _refuseWith = null;
    await tester.tap(find.text('保存'));
    await _settle(tester);
    expect(_ops.single['route'], 'direct');
    expect(app.sheet, isNull);
  });

  testWidgets('我的规则 opens a rule to be edited', (tester) async {
    final app = _app();
    await tester.pumpWidget(_host(app, MyRulesPage(app: app)));
    await _settle(tester);
    await tester.tap(find.text('only.example.com'));
    await _settle(tester);
    expect(find.text('编辑规则'), findsOneWidget);
    await tester.tap(find.text('保存'));
    await _settle(tester);
    expect(_ops.single['old'], {'kind': 'suffix', 'value': 'only.example.com', 'route': 'solo', 'service': 'only'});
    expect(_ops.single['route'], 'solo');
  });

  for (final lang in ['zh', 'en', 'ja', 'zh-Hant']) {
    testWidgets('$lang: the edit sheet fits a narrow phone', (tester) async {
      language = lang;
      tester.view.physicalSize = const Size(360, 760);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.reset);
      final app = _app();
      await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'ai', service: 'work')));
      await _settle(tester);
      await tester.tap(find.text('work.example.com'));
      await _settle(tester);
      expect(app.sheet, isNotNull);
      expect(tester.takeException(), isNull);
    });
  }

  testWidgets('only the groups that can change have a rename and delete menu', (tester) async {
    final app = _app();
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'default')));
    await _settle(tester);
    expect(find.bySemanticsLabel('更多'), findsNothing);
    await tester.pumpWidget(const SizedBox());
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'ai')));
    await _settle(tester);
    expect(find.bySemanticsLabel('更多'), findsOneWidget);
  });

  testWidgets('a route group moves up and down the list, the fixed ones stay', (tester) async {
    final app = _app();
    await tester.pumpWidget(_host(app, RoutesPage(app: app)));
    await _settle(tester);
    // default, direct and reject keep their place: only ai and solo move.
    expect(find.bySemanticsLabel('上移'), findsNWidgets(2));
    expect(find.bySemanticsLabel('下移'), findsNWidgets(2));
    await tester.tap(find.bySemanticsLabel('下移').first);
    await _settle(tester);
    expect(_ops.single, allOf(containsPair('op', 'moveGroup'), containsPair('id', 'ai'), containsPair('to', 'solo')));
    _ops.clear();
    await tester.tap(find.bySemanticsLabel('上移').last);
    await _settle(tester);
    expect(_ops.single, allOf(containsPair('op', 'moveGroup'), containsPair('id', 'solo'), containsPair('to', 'ai')));
    expect(find.byType(GroupPage), findsNothing);
  });

  testWidgets('the first group that can move has nowhere to go up, the last nowhere down', (tester) async {
    final app = _app();
    await tester.pumpWidget(_host(app, RoutesPage(app: app)));
    await _settle(tester);
    await tester.tap(find.bySemanticsLabel('上移').first);
    await tester.tap(find.bySemanticsLabel('下移').last);
    await _settle(tester);
    expect(_ops, isEmpty);
    // A press on an arrow that does nothing doesn't open the group either.
    expect(find.byType(GroupPage), findsNothing);
  });

  testWidgets('a group with one service does not offer to split it', (tester) async {
    final app = _app();
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'solo', service: 'only')));
    await _settle(tester);
    await tester.tap(find.text('更多'));
    await _settle(tester);
    expect(find.text('单独成组'), findsNothing);
    expect(find.text('移到其他路由组'), findsOneWidget);
  });

  testWidgets('adding a service offers those of the other groups', (tester) async {
    final app = _app();
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'solo')));
    await _settle(tester);
    await tester.tap(find.text('添加服务'));
    await _settle(tester);
    expect(find.text('新建服务'), findsOneWidget);
    await tester.tap(find.text('OpenAI'));
    await _settle(tester);
    expect(_ops.single, containsPair('op', 'moveService'));
    expect(_ops.single, containsPair('id', 'openai'));
    expect(_ops.single, containsPair('to', 'solo'));
  });

  /// Opens 导入规则集 from a group's 添加服务.
  Future<AppState> importSheet(WidgetTester tester, {String group = 'solo'}) async {
    final app = _app();
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: group)));
    await _settle(tester);
    await tester.tap(find.text('添加服务'));
    await _settle(tester);
    await tester.tap(find.text('导入规则集'));
    await _settle(tester);
    return app;
  }

  Future<void> paste(WidgetTester tester, String link) async {
    await tester.enterText(find.byType(EditableText), link);
    await _settle(tester);
  }

  List<String> called() => [for (final c in _setCalls) c.$1];

  testWidgets('a rule set is checked and shown before it is imported into the group', (tester) async {
    final app = await importSheet(tester);
    await tester.tap(find.text('导入'));
    await _settle(tester);
    expect(_setCalls, isEmpty, reason: 'nothing was checked yet');
    await paste(tester, 'https://rules.example.com/ChinaDNS.yaml');
    expect(find.text('4 条规则 · 来自 rules.example.com'), findsOneWidget);
    for (final r in _dnsRules) {
      expect(find.text(r), findsOneWidget);
    }
    expect(find.text('命中的流量走「单项」。优先级：你的规则 → 规则集 → 内置规则。'), findsOneWidget);
    await tester.tap(find.text('导入'));
    await _settle(tester);
    expect(called(), ['checkRuleSet', 'importRuleSet']);
    expect(_setCalls.last.$2, containsPair('route', 'solo'));
    expect(_setCalls.last.$2, containsPair('input', 'https://rules.example.com/ChinaDNS.yaml'));
    expect(app.sheet, isNull);
  });

  testWidgets('a source that is no rule list says why and is not imported', (tester) async {
    _checkFails = '没有找到可用的规则，请检查这是不是规则集';
    final app = await importSheet(tester);
    await paste(tester, 'https://rules.example.com/page.html');
    expect(find.text('没有找到可用的规则，请检查这是不是规则集'), findsOneWidget);
    await tester.tap(find.text('导入'));
    await _settle(tester);
    expect(called(), ['checkRuleSet']);
    expect(app.sheet, isNotNull);
  });

  testWidgets('a whole config offers its targets, and one of them is imported', (tester) async {
    await importSheet(tester);
    await paste(tester, 'https://rules.example.com/whole.yaml');
    expect(find.textContaining('这是一份完整配置'), findsOneWidget);
    expect(find.text('DIRECT'), findsOneWidget);
    expect(find.text('2 条规则，跳过 2 条不支持的'), findsOneWidget);
    await tester.tap(find.text('导入'));
    await _settle(tester);
    expect(called(), ['checkRuleSet'], reason: 'no target is chosen yet');
    await tester.tap(find.text('Proxy'));
    await _settle(tester);
    expect(_setCalls.last.$2, containsPair('text', 'Proxy'));
    expect(find.text('「Proxy」的 2 条规则，跳过 2 条不支持的 · 来自 rules.example.com'), findsOneWidget);
    await tester.tap(find.text('导入'));
    await _settle(tester);
    expect(called(), ['checkRuleSet', 'checkRuleSet', 'importRuleSet']);
    expect(_setCalls.last.$2, containsPair('text', 'Proxy'));
    expect(_setCalls.last.$2, containsPair('input', 'https://rules.example.com/whole.yaml'));
  });

  testWidgets('a failed target check can retry or choose another target', (tester) async {
    await importSheet(tester);
    await paste(tester, 'https://rules.example.com/whole.yaml');
    _checkFails = 'HTTP 500';
    await tester.tap(find.text('Proxy'));
    await _settle(tester);
    expect(find.text('HTTP 500'), findsOneWidget);
    expect(find.text('选其他目标'), findsOneWidget);
    _checkFails = null;
    await tester.tap(find.text('重试'));
    await _settle(tester);
    expect(find.text('「Proxy」的 2 条规则，跳过 2 条不支持的 · 来自 rules.example.com'), findsOneWidget);
    await tester.tap(find.text('选其他目标'));
    await _settle(tester);
    expect(find.text('DIRECT'), findsOneWidget);
  });

  testWidgets('a failed import can recheck and import without retyping', (tester) async {
    final app = await importSheet(tester);
    await paste(tester, 'https://rules.example.com/ChinaDNS.yaml');
    _importFails = 'Core refused the config';
    await tester.tap(find.text('导入'));
    await _settle(tester);
    expect(find.text('Core refused the config'), findsOneWidget);
    _importFails = null;
    await tester.tap(find.text('重试'));
    await _settle(tester);
    expect(find.text('4 条规则 · 来自 rules.example.com'), findsOneWidget);
    await tester.tap(find.text('导入'));
    await _settle(tester);
    expect(app.sheet, isNull);
  });

  testWidgets('an imported target still lets another target be chosen', (tester) async {
    await importSheet(tester);
    await paste(tester, 'https://rules.example.com/whole.yaml');
    _targetImported = true;
    await tester.tap(find.text('Proxy'));
    await _settle(tester);
    expect(find.text('已经导入到「直连」了'), findsOneWidget);
    await tester.tap(find.text('选其他目标'));
    await _settle(tester);
    await tester.tap(find.text('DIRECT'));
    await _settle(tester);
    expect(find.text('「DIRECT」的 2 条规则，跳过 2 条不支持的 · 来自 rules.example.com'), findsOneWidget);
  });

  testWidgets('a malformed rule-set URL does not break the rule sheet', (tester) async {
    final app = _app();
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'ai', service: 'work')));
    await _settle(tester);
    await tester.tap(find.text('添加规则').last);
    await _settle(tester);
    await paste(tester, 'http://[::1/rules.yaml');
    expect(tester.takeException(), isNull);
    expect(find.text('Invalid address'), findsOneWidget);
  });

  testWidgets('a link that is already imported says where', (tester) async {
    await importSheet(tester);
    await paste(tester, 'https://rules.example.com/dup.yaml');
    expect(find.text('已经导入到「直连」了'), findsOneWidget);
    await tester.tap(find.text('导入'));
    await _settle(tester);
    expect(called(), ['checkRuleSet']);
  });

  testWidgets('a picked file is checked the way a link is', (tester) async {
    await importSheet(tester);
    await tester.tap(find.text('选择文件'));
    await _settle(tester);
    expect(_setCalls.single.$2, containsPair('path', '/cache/picked/1/ChinaDNS.yaml'));
    expect(find.text('4 条规则 · 来自 ChinaDNS.yaml'), findsOneWidget);
    await tester.tap(find.text('导入'));
    await _settle(tester);
    expect(_setCalls.last.$1, 'importRuleSet');
    expect(_setCalls.last.$2, containsPair('path', '/cache/picked/1/ChinaDNS.yaml'));
  });

  testWidgets('a rule set shows what it holds, counts in the group, and takes no rules of its own', (tester) async {
    _sets = [_ruleSet('set1', 'ChinaDNS', skipped: 1)];
    final app = _app();
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'direct', service: 'set1')));
    await _settle(tester);
    expect(find.textContaining('1 个服务 · 4 条规则'), findsOneWidget);
    expect(find.text('规则集'), findsOneWidget);
    expect(find.text('4 条规则 · 2 小时前更新 · rules.example.com'), findsOneWidget);
    expect(find.text('doh.pub'), findsOneWidget);
    expect(find.text('有 1 条规则不支持，已跳过。'), findsOneWidget);
    expect(find.text('添加规则'), findsNothing);
    expect(find.bySemanticsLabel('删除规则'), findsNothing);
    await tester.tap(find.text('更新'));
    await _settle(tester);
    expect(_setCalls.single.$1, 'updateRuleSet');
    expect(_setCalls.single.$2, containsPair('id', 'set1'));
  });

  testWidgets('a rule set whose update failed keeps its rules and says so', (tester) async {
    _sets = [_ruleSet('set1', 'ChinaDNS', error: 'HTTP 404')];
    final app = _app();
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'direct', service: 'set1')));
    await _settle(tester);
    expect(find.text('更新失败，继续使用上次的规则'), findsOneWidget);
    expect(find.text('更新失败，继续使用上次的 4 条规则：HTTP 404'), findsOneWidget);
    expect(find.text('doh.pub'), findsOneWidget);
  });

  testWidgets('a rule set from a file has no update', (tester) async {
    _sets = [_ruleSet('set1', 'ChinaDNS', local: true)];
    final app = _app();
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'direct', service: 'set1')));
    await _settle(tester);
    expect(find.text('4 条规则 · 本地文件 · ChinaDNS.yaml'), findsOneWidget);
    expect(find.text('更新'), findsNothing);
  });

  testWidgets('a rule-set link typed as a rule points to 导入规则集', (tester) async {
    final app = _app();
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'ai', service: 'work')));
    await _settle(tester);
    await tester.tap(find.text('添加规则').last);
    await _settle(tester);
    await paste(tester, 'https://rules.example.com/ChinaDNS.yaml');
    expect(find.text('这像是规则集的链接。添加规则只会加上域名 rules.example.com，不会用里面的规则。'), findsOneWidget);
    await tester.tap(find.text('导入规则集'));
    await _settle(tester);
    expect(_ops, isEmpty, reason: 'no domain rule was added');
    expect(_setCalls.single.$1, 'checkRuleSet');
    expect(_setCalls.single.$2, containsPair('input', 'https://rules.example.com/ChinaDNS.yaml'));
    expect(find.text('4 条规则 · 来自 rules.example.com'), findsOneWidget);
  });

  for (final lang in ['zh', 'en', 'ja', 'zh-Hant']) {
    testWidgets('$lang: rule sets fit a narrow phone, open and in the import sheet', (tester) async {
      language = lang;
      tester.view.physicalSize = const Size(360, 760);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.reset);
      for (final set in [
        _ruleSet('set1', 'ChinaDNS', skipped: 12, count: 120),
        _ruleSet('set1', 'ChinaDNS', error: 'Get "https://rules.example.com/ChinaDNS.yaml": context deadline exceeded'),
        _ruleSet('set1', 'ChinaDNS', have: false),
        _ruleSet('set1', 'ChinaDNS', local: true, target: 'Proxy'),
        _ruleSet('set1', 'ChinaDNS', off: true),
      ]) {
        _sets = [set];
        final app = _app();
        await tester.pumpWidget(const SizedBox());
        await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'direct', service: 'set1')));
        await _settle(tester);
        expect(tester.takeException(), isNull, reason: '${set['set']}');
      }
      for (final link in ['https://rules.example.com/ChinaDNS.yaml', 'https://rules.example.com/whole.yaml', 'https://rules.example.com/dup.yaml']) {
        await tester.pumpWidget(const SizedBox());
        final app = _app();
        await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'ai')));
        await _settle(tester);
        await tester.tap(find.text(tr('添加服务', 'Add a service')));
        await _settle(tester);
        await tester.tap(find.text(tr('导入规则集', 'Import rule set')));
        await _settle(tester);
        await paste(tester, link);
        expect(tester.takeException(), isNull, reason: link);
      }
    });
  }

  testWidgets('a field opened beside a nested navigator takes the focus', (tester) async {
    final open = ValueNotifier(false);
    await tester.pumpWidget(Localizations(
      locale: const Locale('en'),
      delegates: const [DefaultMaterialLocalizations.delegate, DefaultWidgetsLocalizations.delegate],
      child: MediaQuery(
        data: const MediaQueryData(size: Size(360, 760)),
        child: Skin(
          look: Look.of('paper', false),
          child: Material(
            type: MaterialType.transparency,
            child: Overlay(initialEntries: [
              OverlayEntry(
                builder: (_) => ValueListenableBuilder(
                  valueListenable: open,
                  builder: (context, shown, _) => Stack(children: [
                    Navigator(onGenerateRoute: (_) => PageRouteBuilder<void>(pageBuilder: (_, _, _) => const SizedBox.expand())),
                    if (shown) const BareField(hint: 'name', autofocus: true),
                  ]),
                ),
              ),
            ]),
          ),
        ),
      ),
    ));
    await _settle(tester);
    open.value = true;
    await _settle(tester);
    expect(FocusManager.instance.primaryFocus?.context?.findAncestorWidgetOfExactType<BareField>(), isNotNull);
  });

  testWidgets('a route group other than 直连 and 拦截 can go direct, as on the desktop', (tester) async {
    final app = _app();
    for (final id in ['default', 'direct', 'reject']) {
      await tester.pumpWidget(const SizedBox());
      await tester.pumpWidget(_host(app, GroupPage(app: app, id: id)));
      await _settle(tester);
      expect(find.text('走节点'), findsNothing, reason: id);
    }
    await tester.pumpWidget(const SizedBox());
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'ai')));
    await _settle(tester);
    expect(find.text('节点策略'), findsOneWidget);
    await tester.tap(find.text('直接连接'));
    await _settle(tester);
    expect(_ops.last, {'op': 'setDirect', 'id': 'ai', 'on': true});

    _aiDirect = true;
    app.routing = _routing();
    app.notifyListeners();
    await _settle(tester);
    expect(find.text('节点策略'), findsNothing);
    expect(find.textContaining('这个路由组的流量用本机网络访问'), findsOneWidget);
    expect(find.bySemanticsLabel('更多'), findsOneWidget, reason: 'it can still be renamed and deleted');
    expect(find.text('服务 · 2'), findsOneWidget, reason: 'its services stay');
    await tester.tap(find.text('走节点'));
    await _settle(tester);
    expect(_ops.last, {'op': 'setDirect', 'id': 'ai', 'on': false});
  });

  testWidgets('a route group can lock its exit, asked the first time, as on the desktop', (tester) async {
    final app = _app();
    for (final id in ['default', 'direct', 'reject']) {
      await tester.pumpWidget(const SizedBox());
      await tester.pumpWidget(_host(app, GroupPage(app: app, id: id)));
      await _settle(tester);
      expect(find.text('出口锁定'), findsNothing, reason: id);
    }
    await tester.pumpWidget(const SizedBox());
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'ai')));
    await _settle(tester);
    expect(find.text('只走节点，不改走直连'), findsOneWidget);
    await tester.tap(find.text('出口锁定'));
    await _settle(tester);
    expect(_ops, isEmpty, reason: 'the first lock is asked first');
    expect(find.text('锁定「AI 服务」的出口'), findsOneWidget);
    expect(find.textContaining('关闭 VPN 后 bowel 拦不住它'), findsOneWidget, reason: "it says what the phone can't do");
    await tester.tap(find.text('锁定'));
    await _settle(tester);
    expect(_ops.last, {'op': 'setLocked', 'id': 'ai', 'on': true});

    _aiLocked = true;
    app.routing = _routing();
    app.notifyListeners();
    await _settle(tester);
    expect(find.textContaining('已锁定 · '), findsOneWidget);
    final before = _ops.length;
    await tester.tap(find.text('直接连接'));
    await _settle(tester);
    expect(_ops.length, before, reason: "a locked group doesn't go direct");
    await tester.tap(find.text('出口锁定'));
    await _settle(tester);
    expect(_ops.last, {'op': 'setLocked', 'id': 'ai', 'on': false}, reason: 'unlocking is a tap');

    await tester.pumpWidget(const SizedBox());
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'solo')));
    await _settle(tester);
    await tester.tap(find.text('出口锁定'));
    await _settle(tester);
    expect(_ops.last, {'op': 'setLocked', 'id': 'solo', 'on': true}, reason: 'a lock after the first is a tap');

    await tester.pumpWidget(const SizedBox());
    await tester.pumpWidget(_host(app, RoutesPage(app: app)));
    await _settle(tester);
    expect(find.bySemanticsLabel('出口锁定'), findsOneWidget, reason: 'the list marks the locked group');
    expect(tester.takeException(), isNull);
  });

  Future<void> openAppSheet(WidgetTester tester, AppState app, {double width = 360, double height = 760, double keyboard = 0}) async {
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'ai', service: 'work'), width: width, height: height, keyboard: keyboard));
    await _settle(tester);
    await tester.tap(find.text('添加规则').last);
    await _settle(tester);
    await tester.tap(find.widgetWithText(Chip, '应用'));
    await _settle(tester);
  }

  testWidgets('应用 shows next to 自动识别, on screen on a narrow phone', (tester) async {
    _appKind = true;
    tester.view.physicalSize = const Size(360, 760);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final app = _app();
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'ai', service: 'work')));
    await _settle(tester);
    await tester.tap(find.text('添加规则').last);
    await _settle(tester);
    final detect = tester.getRect(find.widgetWithText(Chip, '自动识别')), apps = tester.getRect(find.widgetWithText(Chip, '应用'));
    expect(apps.left > detect.left && apps.left - detect.right < 10, isTrue, reason: '应用 at $apps, 自动识别 at $detect');
    expect(apps.right <= 360, isTrue, reason: 'no scrolling to find it');
    expect(tester.takeException(), isNull);
  });

  testWidgets('with the keyboard up, a long app list gives way and Add stays on screen', (tester) async {
    _appKind = true;
    _installed = [for (var i = 0; i < 30; i++) {'id': 'com.example.app$i', 'name': 'App $i'}];
    // The phone it was seen on: 412 × 898 dp, its keyboard 350 dp tall.
    tester.view.physicalSize = const Size(412, 898);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final app = _app();
    await openAppSheet(tester, app, width: 412, height: 898, keyboard: 350);
    expect(tester.takeException(), isNull);
    final add = tester.getRect(find.text('添加').last);
    expect(add.bottom <= 898 - 350, isTrue, reason: 'Add at $add, above the keyboard');
    expect(find.text('App 0'), findsOneWidget, reason: 'the list is there, shorter');
  });

  testWidgets('a typed package bowel cannot see says the rule would not match, and still takes it', (tester) async {
    _appKind = true;
    final app = _app();
    await openAppSheet(tester, app);
    await tester.enterText(find.byType(EditableText), _hidden);
    await _settle(tester);
    expect(find.text('bowel 看不到这个 App：没装，或者系统对其他 App 隐藏了它。看不到时，这条规则不会生效。'), findsOneWidget);
    expect(find.text('列表里没有这个包名，也可以照样添加。'), findsNothing);
    await tester.tap(find.text('添加').last);
    await _settle(tester);
    expect(_ops.single['text'], _hidden, reason: 'it may be installed, or shown, later');
  });

  testWidgets('an app is picked from the installed ones and its package becomes the rule', (tester) async {
    _appKind = true;
    final app = _app();
    await openAppSheet(tester, app);
    expect(find.text('ChatGPT'), findsOneWidget);
    expect(find.text('Gmail'), findsOneWidget);
    await tester.enterText(find.byType(EditableText), 'chat');
    await _settle(tester);
    expect(find.text('Gmail'), findsNothing, reason: 'the field searches the list');
    await tester.tap(find.text('ChatGPT'));
    await _settle(tester);
    expect(tester.widget<EditableText>(find.byType(EditableText)).controller.text, 'com.openai.chatgpt');
    expect(find.text('添加后：com.openai.chatgpt · 应用'), findsOneWidget);
    await tester.tap(find.text('添加').last);
    await _settle(tester);
    expect(_ops.single, {'op': 'addRule', 'kind': 'app', 'text': 'com.openai.chatgpt', 'route': 'ai', 'service': 'work'});
    expect(app.sheet, isNull);
  });

  testWidgets('a search that is no package is not added as an app', (tester) async {
    _appKind = true;
    final app = _app();
    await openAppSheet(tester, app);
    await tester.enterText(find.byType(EditableText), 'chat');
    await _settle(tester);
    await tester.tap(find.text('添加').last);
    await _settle(tester);
    expect(_ops, isEmpty);
    expect(find.text('选一个 App，或写完整的包名'), findsOneWidget);
    await tester.enterText(find.byType(EditableText), 'org.example.sync');
    await _settle(tester);
    expect(find.text('列表里没有这个包名，也可以照样添加。'), findsOneWidget);
    await tester.tap(find.text('添加').last);
    await _settle(tester);
    expect(_ops.single['text'], 'org.example.sync', reason: 'a package not in the launcher list still takes a rule');
  });

  testWidgets('an app kept out of the VPN takes no rule, and says why', (tester) async {
    _appKind = true;
    final app = _app()..vpnExcluded = 'com.google.android.gm';
    await openAppSheet(tester, app);
    expect(find.text('com.google.android.gm · 不走 bowel'), findsOneWidget);
    await tester.tap(find.text('Gmail'));
    await _settle(tester);
    final why = find.textContaining('Gmail 已设为不走 bowel');
    expect(why, findsOneWidget);
    expect(tester.widget<EditableText>(find.byType(EditableText)).controller.text, isEmpty, reason: 'it is not picked');
    await tester.enterText(find.byType(EditableText), 'com.google.android.gm');
    await _settle(tester);
    expect(why, findsOneWidget, reason: 'typed, it says so at once');
    expect(find.textContaining('添加后：'), findsNothing);
    await tester.tap(find.text('添加').last);
    await _settle(tester);
    expect(_ops, isEmpty, reason: 'and is refused');
    expect(why, findsOneWidget);
  });

  testWidgets('a rule for an app kept out of the VPN says it never matches', (tester) async {
    _appKind = true;
    _myAiRules = [_appRule('com.google.android.gm'), _appRule('com.openai.chatgpt')];
    final app = _app()..vpnExcluded = 'com.google.android.gm';
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'ai', service: 'mine:ai')));
    await _settle(tester);
    expect(find.text('应用 · 已设为不走 bowel，重新连接后规则不生效'), findsOneWidget, reason: 'only the one kept out');
    await tester.pumpWidget(const SizedBox());
    await tester.pumpWidget(_host(app, MyRulesPage(app: app)));
    await _settle(tester);
    expect(find.text('应用 · AI 服务 · 已设为不走 bowel，重新连接后规则不生效'), findsOneWidget);
    expect(find.text('应用 · AI 服务'), findsOneWidget);
  });

  testWidgets('a rule for an app opens to be edited with the app picked', (tester) async {
    _appKind = true;
    _myAiRules = [_appRule('com.openai.chatgpt')];
    final app = _app();
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'ai', service: 'mine:ai')));
    await _settle(tester);
    await tester.tap(find.text('com.openai.chatgpt'));
    await _settle(tester);
    expect(find.text('编辑规则'), findsOneWidget);
    expect(find.text('ChatGPT'), findsOneWidget);
    expect(find.text('保存后：com.openai.chatgpt · 应用'), findsOneWidget);
    await tester.tap(find.text('保存'));
    await _settle(tester);
    expect(_ops.single['op'], 'editRule');
    expect([_ops.single['kind'], _ops.single['text']], ['app', 'com.openai.chatgpt']);
  });

  testWidgets('without rules for apps, as before Android 10, there is no app kind and its rules only go', (tester) async {
    _myAiRules = [_appRule('com.openai.chatgpt')];
    final app = _app();
    await tester.pumpWidget(_host(app, GroupPage(app: app, id: 'ai', service: 'mine:ai')));
    await _settle(tester);
    await tester.tap(find.text('com.openai.chatgpt'));
    await _settle(tester);
    expect(app.sheet, isNull, reason: "it doesn't open to be edited");
    await tester.tap(find.text('添加规则').first);
    await _settle(tester);
    expect(find.text('添加'), findsWidgets, reason: 'the sheet is open');
    final chips = find.ancestor(of: find.widgetWithText(Chip, '自动识别'), matching: find.byType(Scrollable)).first;
    await tester.drag(chips, const Offset(-600, 0));
    await _settle(tester);
    expect(find.widgetWithText(Chip, 'IP 段'), findsOneWidget, reason: 'the end of the kinds');
    expect(find.widgetWithText(Chip, '应用'), findsNothing);
  });
}
