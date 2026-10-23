package __JAVA_PACKAGE__;

import android.app.Activity;
import android.os.Bundle;
import android.view.ViewGroup;
import android.widget.FrameLayout;

public final class MainActivity extends Activity {
    private OnixSurfaceView surfaceView;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
		super.onCreate(savedInstanceState);
		surfaceView = new OnixSurfaceView(this);
		FrameLayout rootView = new FrameLayout(this);
		rootView.addView(surfaceView, new FrameLayout.LayoutParams(
			ViewGroup.LayoutParams.MATCH_PARENT,
			ViewGroup.LayoutParams.MATCH_PARENT
		));
		NativeBridge.initPlatformServices(rootView);
        setContentView(rootView);
    }

    @Override
    protected void onResume() {
        super.onResume();
        surfaceView.onResume();
    }

    @Override
	protected void onPause() {
        surfaceView.onPause();
		super.onPause();
	}

	@Override
	protected void onDestroy() {
		NativeBridge.shutdownPlatformServices();
		super.onDestroy();
	}

}
