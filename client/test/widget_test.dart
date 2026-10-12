import 'package:flutter_test/flutter_test.dart';
import 'package:sameframe_client/src/app.dart';

void main() {
  testWidgets('renders the lobby entry state', (tester) async {
    await tester.pumpWidget(const SameFrameApp());
    expect(find.text('SAMEFRAME'), findsOneWidget);
    expect(find.text('创建同帧账号'), findsOneWidget);
    expect(find.text('注册并进入'), findsOneWidget);
  });
}
