import 'package:flutter_test/flutter_test.dart';
import 'package:sameframe_client/src/app.dart';

void main() {
  testWidgets('renders the lobby entry state', (tester) async {
    await tester.pumpWidget(const SameFrameApp());
    expect(find.text('SAMEFRAME'), findsOneWidget);
    expect(find.text('创建同帧账号'), findsOneWidget);
    expect(find.text('邮箱验证码'), findsNothing);
    expect(find.text('邮箱'), findsOneWidget);
    expect(find.text('发现公开房间'), findsNothing);
  });
}
