import 'package:flutter_test/flutter_test.dart';
import 'package:sameframe_client/src/app.dart';

void main() {
  testWidgets('renders the lobby entry state', (tester) async {
    await tester.pumpWidget(const SameFrameApp());
    expect(find.text('SameFrame · 同帧'), findsOneWidget);
    expect(find.text('进入测试环境'), findsOneWidget);
    expect(find.text('进入大厅'), findsOneWidget);
  });
}
