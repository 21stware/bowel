import 'package:bowel/app/app_list.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('reads one package per line', () {
    expect(parsePackages('com.tencent.mm\ncom.eg.android.AlipayGphone\r\n\n'), {'com.tencent.mm', 'com.eg.android.AlipayGphone'});
  });

  test('takes commas, spaces and full-width separators', () {
    expect(parsePackages('com.a.b, com.c.d；com.e_f.g  tv.danmaku.bili，com.a.b'), {'com.a.b', 'com.c.d', 'com.e_f.g', 'tv.danmaku.bili'});
  });

  test('picks the names out of JSON', () {
    expect(parsePackages('{"mode":"rejectSelected","rejectList":["com.tencent.mm","com.taobao.taobao"]}'), {'com.tencent.mm', 'com.taobao.taobao'});
  });

  test('skips words that are not package names', () {
    expect(parsePackages('hello world 1.2.3 com'), isEmpty);
  });

  test('writes them sorted, one per line', () {
    expect(packagesText({'com.z.z', '', 'com.a.a'}), 'com.a.a\ncom.z.z');
    expect(parsePackages(packagesText({'com.z.z', 'com.a.a'})), {'com.z.z', 'com.a.a'});
  });
}
