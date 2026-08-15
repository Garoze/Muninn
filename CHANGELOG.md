# Changelog

## [0.2.1-alpha](https://github.com/Garoze/Muninn/compare/v0.2.0...v0.2.1-alpha) (2026-08-15)


### Features

* add ClusterIP Service for in-cluster access to Muninn's gRPC API ([cd77cad](https://github.com/Garoze/Muninn/commit/cd77cad752680232e2d2e48985c91b18c9606fb9))
* add config/manager and config/rbac for in-cluster deployment ([b45bf09](https://github.com/Garoze/Muninn/commit/b45bf0902fdc7a3c08bdc53359a81d9b715f200e))
* add config/webhook/ manifests for the mutating admission webhook ([e0d3d23](https://github.com/Garoze/Muninn/commit/e0d3d23cc960aba677aaa1b6c43643660844a4e9))
* add discoveryclient, a shared gRPC dial helper ([b3bd74a](https://github.com/Garoze/Muninn/commit/b3bd74ab23b4d5cc3606ef25d1cfbd8de4a43c92))
* add muninn resolve, the CLI mode backing the webhook's init container/sidecar ([9e71e4c](https://github.com/Garoze/Muninn/commit/9e71e4c2391cb2f48c958d9e949b78393399e5dd))
* add Resolve to the discovery.v1 proto ([72a20bc](https://github.com/Garoze/Muninn/commit/72a20bc95adfe0ec335519bef5ab9b8eaaf4550e))
* add sample fixtures and complete the sample Makefile target ([65bfbe4](https://github.com/Garoze/Muninn/commit/65bfbe45fcdbb675a5a1228e656afbf7f3e2e4e4))
* add the mutating admission webhook server ([1dbc52a](https://github.com/Garoze/Muninn/commit/1dbc52ada08c9cf38486a2e3c62da6ebc68c90e1))
* anchor the injected image env var to the Deployment's own image ([deec0a7](https://github.com/Garoze/Muninn/commit/deec0a706024f2e7bdfdec0be5c78d07b4818f8f))
* **api:** add discovery.proto with Query and Describe RPCs ([1d347a5](https://github.com/Garoze/Muninn/commit/1d347a57ce2bd25bb5f5c8c0a904498b8e872289))
* **api:** add Policy CRD type definition ([e8573ea](https://github.com/Garoze/Muninn/commit/e8573eaf35405d466c6cd8a8189fcb214525f281))
* **api:** add Tenant CRD type definition ([e6f4bb3](https://github.com/Garoze/Muninn/commit/e6f4bb3d2aa8caeac6d8ec0581cb36e07bcded44))
* **api:** add TenantConfig CRD type definition ([06c3c81](https://github.com/Garoze/Muninn/commit/06c3c81b08669fa824ee33f88460f498731f270c))
* **api:** add v1alpha1 package doc and GroupVersion schema registration ([255adc1](https://github.com/Garoze/Muninn/commit/255adc1cc6e4e10621857ab986989904643d8b65))
* **app:** add Cache and DiscoveryService with key whitelisting ([1e18c68](https://github.com/Garoze/Muninn/commit/1e18c68c4bb4318a5cf01913276c2d97500f6ed8))
* **app:** add Resolve to the domain layer ([c1d2848](https://github.com/Garoze/Muninn/commit/c1d2848c40cf846f232832917659435fa545bd17))
* **app:** add sentinel errors ([e7779f6](https://github.com/Garoze/Muninn/commit/e7779f68d4ef278fe305f31fbd025ffc3af9a1d6))
* **build:** add ko configuration ([46b56d9](https://github.com/Garoze/Muninn/commit/46b56d9f4b0131e579ff0ebecbd42f90085c025d))
* **build:** stamp muninn/muninnctl binaries with a build-time version ([6957e5c](https://github.com/Garoze/Muninn/commit/6957e5ca22ece4a16bee7f7ff5442e391f9a8a3a))
* **chart:** add helm-unittest, kubeconform, and ct lint to CI ([dce9ad7](https://github.com/Garoze/Muninn/commit/dce9ad70393520b9bd2a4f70d330b6a995823297))
* **chart:** add opt-in cert-manager/CSI-driver dependencies and a webhook enable toggle ([0fbca89](https://github.com/Garoze/Muninn/commit/0fbca89e2f38a9026dbc8122e9dc5cc409a92456))
* **chart:** add the Helm chart's mechanical translation and core values ([cf493a6](https://github.com/Garoze/Muninn/commit/cf493a6928d238854f4b5a7ea3940920934bbc79))
* **ci:** add a nightly workflow installing the published chart and image ([b3b80bd](https://github.com/Garoze/Muninn/commit/b3b80bd227985cfaaa4f4e8d20ccf70e3f9a7c14))
* **ci:** consolidate the changelog and back-merge main into develop ([85b222e](https://github.com/Garoze/Muninn/commit/85b222ea445c3eff5c312a65cfd47fcf65a91f07))
* **ci:** publish and sign the Helm chart alongside the image ([4bb9d3b](https://github.com/Garoze/Muninn/commit/4bb9d3bd3add95018c0963c93e5129b784cf3f6a))
* **cmd:** add muninnctl CLI ([e455fad](https://github.com/Garoze/Muninn/commit/e455fad82f7319df01a5bb3737713b318fa9e500))
* **cmd:** turn muninn into a multi-mode CLI (serve/webhook/resolve) ([80b7ec4](https://github.com/Garoze/Muninn/commit/80b7ec45c95027f2eead712621bcd3b734906101))
* **config:** add ENABLED_CONFIG_SOURCES to filter registered sources ([729824a](https://github.com/Garoze/Muninn/commit/729824a8acd41b26b073a80b134328026d1d14c5))
* **config:** add env-based config struct ([7222eda](https://github.com/Garoze/Muninn/commit/7222edac326ae223ccdeb43556d8391fdd99841b))
* **config:** add InjectImage for the webhook's injected containers ([55097c9](https://github.com/Garoze/Muninn/commit/55097c93ae1960df78f7e08433c47badd64e81c8))
* **config:** add RBAC and manifests for CSI secret delivery ([448d7dc](https://github.com/Garoze/Muninn/commit/448d7dce9ec1c85178bbc0fd501b32c9aef9d9fb))
* **config:** add SecretProviderClass mode and Vault settings ([f6a652b](https://github.com/Garoze/Muninn/commit/f6a652b3d606c4f66fe2931b567638b2ff57824e))
* **config:** add SelfAddr for injected consumers to reach Muninn ([0788efa](https://github.com/Garoze/Muninn/commit/0788efa117406abc10cb9fb76217648cc251d8c5))
* **config:** add webhook TLS/bind config fields ([5d61a62](https://github.com/Garoze/Muninn/commit/5d61a62adf634cd0f6389f7f29f047ab9d350a31))
* **config:** make gRPC API TLS configurable and optional ([55d9221](https://github.com/Garoze/Muninn/commit/55d92215b94f03a8f5688739beb23953721955b1))
* **config:** update RBAC and sample fixtures for ConfigMap watching ([2f9e8f5](https://github.com/Garoze/Muninn/commit/2f9e8f510706731d7dbf5576687e61721a26b697))
* **deps:** add Dockerfile and .dockerignore for container builds ([eaee89e](https://github.com/Garoze/Muninn/commit/eaee89eb27d04abbd30a95f85ef00e2e9c05ef0a))
* **kube:** add CRD watcher with patch-based cache sync ([2d371c4](https://github.com/Garoze/Muninn/commit/2d371c47d2277f1973af28ff40f5b650cf31e84d))
* **kube:** add pluggable ConfigSource interface for bring-your-own CRDs ([ffd8446](https://github.com/Garoze/Muninn/commit/ffd8446b669d82d2c54e801e5b912ad8d2cb0aa5))
* **kube:** add scheme provider for CRD-aware controller-runtime cache ([203c3e0](https://github.com/Garoze/Muninn/commit/203c3e089c83aedf0287d9af3cdf91f293aa1dd1))
* **kube:** add SecretSource as a second ConfigSource implementation ([4227b01](https://github.com/Garoze/Muninn/commit/4227b016ad2650e30408a193ab7f3f205645e670))
* **kube:** bridge zap logger into controller-runtime ([fba501d](https://github.com/Garoze/Muninn/commit/fba501d21ba6e87cef9ef0327adf49f01a5ef023))
* **kube:** filter registered ConfigSources by ENABLED_CONFIG_SOURCES ([1e72531](https://github.com/Garoze/Muninn/commit/1e725317a2fc18147af18ae4177017e9f5b44bf6))
* **kube:** register SecretProviderClass in the scheme ([74ff8a8](https://github.com/Garoze/Muninn/commit/74ff8a8b56cc20969e57097a78a08b0f2b1ea69f))
* **observability:** add gRPC health server helpers ([325973e](https://github.com/Garoze/Muninn/commit/325973e66c3aff43170f6461752af517a40afe75))
* **observability:** add gRPC server and listener setup ([fa645c3](https://github.com/Garoze/Muninn/commit/fa645c31fd099ac4ce8181f84a977c95408b2f64))
* **observability:** add OpenTelemetry tracing on every gRPC call ([a86e9b7](https://github.com/Garoze/Muninn/commit/a86e9b7eb703001e44be595024533dcb8811a7e4))
* **observability:** add Prometheus metrics ([1343dde](https://github.com/Garoze/Muninn/commit/1343ddee111dd1d7d5d78a78f7cd5faac97ec4fb))
* **observability:** add webhook request/injection metrics ([d705cf7](https://github.com/Garoze/Muninn/commit/d705cf7b40153a5433966123e5b79a7d96c38be9))
* **observability:** export NewLogger ([8b37706](https://github.com/Garoze/Muninn/commit/8b37706ad7b7b7008ca56635305aae8abd315fa9))
* **observability:** rename QueriesTotal to RequestsTotal ([fa27fd7](https://github.com/Garoze/Muninn/commit/fa27fd71c27b852ec2d9265e7ce920a5f5f185bd))
* **proto:** generate gRPC stubs from discovery.proto ([c6974e0](https://github.com/Garoze/Muninn/commit/c6974e0818bba3326330ee002acf83df52517d2f))
* replace Tenant/Policy/TenantConfig CRDs with ConfigMap-based config resolution ([3fefd54](https://github.com/Garoze/Muninn/commit/3fefd54e5f70d1ba15a4cc9781a2217f0034a3b6))
* start the metrics server for muninn webhook ([0b1bcb3](https://github.com/Garoze/Muninn/commit/0b1bcb3ea74d580133820a238d35e17df4f56ca7))
* **transport:** add gRPC DiscoveryHandler with error classification ([c99be0a](https://github.com/Garoze/Muninn/commit/c99be0a3dee5fe7209c83b12b6c6a4b7d8e7a2b4))
* **transport:** add optional TLS support to the shared gRPC dial helper ([149a453](https://github.com/Garoze/Muninn/commit/149a453e4e223b401b176f6c7cc95eb33068e716))
* **transport:** add Resolve gRPC handler ([7fadda7](https://github.com/Garoze/Muninn/commit/7fadda7dc4957764a95a5f79ff97704db28f29c4))
* **webhook:** add a writable client for derived objects ([1aafa0d](https://github.com/Garoze/Muninn/commit/1aafa0d8b76fe025ed73db9d8ddc3d9009692e22))
* **webhook:** add OTel tracing to the /mutate endpoint ([4bbe831](https://github.com/Garoze/Muninn/commit/4bbe831afe7bb01d95f55d6785c16c6905773541))
* **webhook:** add Pod injection patch builder ([b96ccb2](https://github.com/Garoze/Muninn/commit/b96ccb2c1b8edcdb25c7c08b53da6c4b6a3b6c03))
* **webhook:** derive and validate a namespace's SecretProviderClass ([7704130](https://github.com/Garoze/Muninn/commit/7704130706b62cd39aaddbd7a4f9f35c149b5a55))
* **webhook:** extract secret references from resolved config ([a2fe38c](https://github.com/Garoze/Muninn/commit/a2fe38c9a9a779cc57a186543ab63cfc2513df78))
* **webhook:** inject config volume/containers into opted-in Pods ([4eb953c](https://github.com/Garoze/Muninn/commit/4eb953c038cf3a3f1aa6eca484451ff6dd65708d))
* **webhook:** mount the shared volume into the Pod's own containers ([a8de993](https://github.com/Garoze/Muninn/commit/a8de9933d80d4ea9c54a93f4ac55450fae52d520))
* **webhook:** record Prometheus metrics for /mutate ([ef6f99f](https://github.com/Garoze/Muninn/commit/ef6f99fa7c19f4ecc2bedd2e1d0ae313674a5430))
* **webhook:** report new secret references from the sidecar ([5833c68](https://github.com/Garoze/Muninn/commit/5833c68814dfeece4335a873afa236ad7509bd8f))
* **webhook:** resolve in-process and inject the CSI secrets volume ([85a1138](https://github.com/Garoze/Muninn/commit/85a11385de6f734075e1539049c85583bda53d0f))
* wire app, kube, observability, and transport into an Fx graph ([e57d612](https://github.com/Garoze/Muninn/commit/e57d61210087322f471bce227a4272aa4e870ef4))
* wire the tracer provider into muninn webhook ([e65f760](https://github.com/Garoze/Muninn/commit/e65f760f2e101d983e7cae2ad3f7b55ff4afff0d))


### Bug Fixes

* add missing serve arg to the manager Deployment ([d5d7eee](https://github.com/Garoze/Muninn/commit/d5d7eee431265a1fdf3fe5211652b3d9d15a08e8))
* **api:** correct json tag, field name, and marker typos on CRD types ([ed7e04e](https://github.com/Garoze/Muninn/commit/ed7e04e2597b8e7821ea1aa9da94caa6f7640798))
* **api:** correct kubebuilder shortName marker casing ([810a308](https://github.com/Garoze/Muninn/commit/810a3080ea8284d0977999bc53f19908539ef508))
* **api:** correct metav1 import path and use controller-runtime scheme builder ([6fc582b](https://github.com/Garoze/Muninn/commit/6fc582bfbda613d235e64c0024507a41e6669ed5))
* **api:** embed ListMeta instead of ObjectMeta on List types ([d4ef631](https://github.com/Garoze/Muninn/commit/d4ef6310cd1ef7a6ad469cb20f9022d89989d653))
* **api:** enable deepcopy generation for non-root types ([e6333db](https://github.com/Garoze/Muninn/commit/e6333dbbb51794d11fc76eb66d7f11776f58e861))
* **app:** add an atomic cache mutation primitive ([3b78202](https://github.com/Garoze/Muninn/commit/3b78202eedd3f76cf337e2331f4b21aa1347f233))
* **ci:** force fresh test-integration runs, not go test's stale-pass cache ([c6ecc54](https://github.com/Garoze/Muninn/commit/c6ecc54e8cd8f8b9b371de243e421aa43be597c3))
* **ci:** install helm explicitly before chart-testing-action ([5100e16](https://github.com/Garoze/Muninn/commit/5100e16a9966b5b84141c6b2a3b02033187a80ce))
* **ci:** match linked changelog headings when renaming for a release ([805cc38](https://github.com/Garoze/Muninn/commit/805cc38e56e67a3d2cedba3aa9389f9f7cb7ab4e))
* **ci:** register subchart repositories and fetch dependencies ([dcf6476](https://github.com/Garoze/Muninn/commit/dcf64766ec7c128a3b6c90cd0e1204139b7dce6e))
* **ci:** resync develop's release-please manifest after an official cut ([b1106e6](https://github.com/Garoze/Muninn/commit/b1106e6a2cb26e2fed90aaccb674f53dcb12b9aa))
* **cmd:** check the version subcommand's Fprintln error ([f5d1492](https://github.com/Garoze/Muninn/commit/f5d1492f4bec078b28e82016c1e3f49ca0b4e77a))
* **cmd:** dedupe hardcoded pod label in e2e test ([f6967a5](https://github.com/Garoze/Muninn/commit/f6967a5635cbcea9a4a7137620011578c846aacc))
* **cmd:** rephrase a comment in writeFileAtomic ([6d6f8bd](https://github.com/Garoze/Muninn/commit/6d6f8bd5958e9f94f0a6b203272dccbb18f85931))
* **cmd:** satisfy errcheck lint with real error handling in muninnctl ([3a29ed8](https://github.com/Garoze/Muninn/commit/3a29ed8f81adf8f3b988c922cbbaa3fe0b9983a4))
* **config:** correct OTLP exporter default port (4371 -&gt; 4317) ([589a2ef](https://github.com/Garoze/Muninn/commit/589a2efc10fd9ad0cf30d3923399a913b99dba23))
* **config:** correct probe address default and kubeconfig env var ([c0bc5ec](https://github.com/Garoze/Muninn/commit/c0bc5ec170dcb7ec15e7fea60328ed48735b009c))
* **config:** correct TraceSampleRatio field name typo ([3e87625](https://github.com/Garoze/Muninn/commit/3e876257afe06c1f87ef0a47a97ae1b8087fffcd))
* **config:** keep release-please under 1.0.0 and seed the first alpha ([1b4fbe8](https://github.com/Garoze/Muninn/commit/1b4fbe82557df51affcb634b10935e8966551ab4))
* **config:** pass KUBE_CONFIG_PATH from make run, not KUBECONFIG ([c455ba8](https://github.com/Garoze/Muninn/commit/c455ba813811da31d52bbaa23d702995bcef11e4))
* **config:** point the manager/webhook Deployments at the published image ([8edb69b](https://github.com/Garoze/Muninn/commit/8edb69b3494133a1fb1eae58b3a33023408aca63))
* **deps:** make image reference engine-agnostic in make load ([703cf2c](https://github.com/Garoze/Muninn/commit/703cf2c1d3071f81b1bfdb2cac3228f6bebffd7c))
* **deps:** make image/load targets container-engine-agnostic ([7d9539b](https://github.com/Garoze/Muninn/commit/7d9539b9aafa6b95a0411ba33ba1c8512f7a2f2b))
* **kube:** correct Fx wiring for the config_sources value group ([eabc13d](https://github.com/Garoze/Muninn/commit/eabc13d83cbd7c13ffdf609c89041a5bb007ff1c))
* **kube:** correct merge semantics and scope informers per source ([c5bc447](https://github.com/Garoze/Muninn/commit/c5bc4475a7d99543271eb6dc5a465a6548313fa5))
* **kube:** remove entire cache entry on Tenant delete, not just owned fields ([06447d4](https://github.com/Garoze/Muninn/commit/06447d44b967ee5c14510aa2cf6fd0880416ca2f))
* **kube:** unwrap tombstones in extractTenantConfig ([f63558b](https://github.com/Garoze/Muninn/commit/f63558b3267e4500f4f37967bfaef6ff942e5275))
* **kube:** watch TenantConfig informer instead of duplicate Policy ([62904cf](https://github.com/Garoze/Muninn/commit/62904cff02cc6fc4704c9c322911a2841d18b84d))
* **observability:** bind primary gRPC listener to service address ([03104b7](https://github.com/Garoze/Muninn/commit/03104b748a0321332b33c58a2183847f4ffd7601))
* **observability:** check listener Close error in tests ([4697482](https://github.com/Garoze/Muninn/commit/4697482d14d00a4ee8aba692007f3c29fceeb026))
* **observability:** correct QueriesTotal label cardinality ([440de23](https://github.com/Garoze/Muninn/commit/440de239a2cbaf5dc116fd3e658a77fac7d0bbeb))
* **observability:** log the probe server's exit error and fix metric names ([beca2ab](https://github.com/Garoze/Muninn/commit/beca2abf9dcd2c1754f36551de5062081e1e626f))
* remove leftover tenant terminology from cache metrics/config comments ([9c6ac9d](https://github.com/Garoze/Muninn/commit/9c6ac9de88d1cdda6519f449fbb19e3378f33403))
* **transport:** use equality.Semantic.DeepEqual for webhook patch idempotency ([641406f](https://github.com/Garoze/Muninn/commit/641406fe6ab277eeea3084cceb1f0451de8d1465))
* **webhook:** set ImagePullPolicy on injected containers ([6c9a012](https://github.com/Garoze/Muninn/commit/6c9a012a9d93dddd7dbbd7e818e84a3f3898d672))
* **webhook:** stop dropping the injected volume and misrouting initContainers patch ([d8f3459](https://github.com/Garoze/Muninn/commit/d8f3459f177f457bc91d489bdfb9cb40e5b88513))


### Code Refactoring

* **api:** drop controller-runtime dependency from scheme registration ([16185bf](https://github.com/Garoze/Muninn/commit/16185bfcc7354a96c79d0e1e435ae40e279dc8e5))
* **app:** rename logger field to log for consistency ([e8c2a01](https://github.com/Garoze/Muninn/commit/e8c2a015f7e833f57f671a105b7d9932c35b9f00))
* **app:** unexpot logger and TTL fields, inject config into constructor ([1b60edd](https://github.com/Garoze/Muninn/commit/1b60edd8dfbc8a456732d14b9c23c0e3f7853742))
* **cmd:** muninnctl uses the shared discoveryclient.Dial ([94218cc](https://github.com/Garoze/Muninn/commit/94218cc41372cf76f9d138ff4a6ab6c7c9534b51))
* **kube:** fail fast on duplicate ConfigSource KeyPrefix ([e5838f8](https://github.com/Garoze/Muninn/commit/e5838f84e47373226c5962357ce8d81c7c981a89))
* **kube:** remove SecretSource ([165ff2e](https://github.com/Garoze/Muninn/commit/165ff2e677dcd6de8702d049bcaea586a6101833))
* **kube:** split KeyPrefix from Kind on ConfigSource ([3f14d9b](https://github.com/Garoze/Muninn/commit/3f14d9b0d93423cdae1242d70a7961685dbd204a))
* **kube:** tighten KeyPrefix comments ([6c8099b](https://github.com/Garoze/Muninn/commit/6c8099befe8d3f81e1e1c9acc4107d502809c2fb))
* **observability:** export ShutdownTracerProvider ([5396331](https://github.com/Garoze/Muninn/commit/5396331f7155d212b607e0d6002887ff4718a42e))
* **observability:** export StartMetricsServer ([1194fa3](https://github.com/Garoze/Muninn/commit/1194fa3bdaab8be2018a8cd795137444c0ed5f09))
* tighten source comments to terse, WHY-only style ([574fcac](https://github.com/Garoze/Muninn/commit/574fcac72f0ad4e8bac079724bc09f8fa1f27e02))
* **transport:** move gRPC server/listener/TLS construction out of observability ([4e223c1](https://github.com/Garoze/Muninn/commit/4e223c1f19c8ad0cf96d99eeb958884d239ee762))


### Documentation

* add a guide for writing a config source ([e9317a5](https://github.com/Garoze/Muninn/commit/e9317a59131c65428ab280b56921392e7bd4ca24))
* add CI, license, and Go version badges ([e85dbe5](https://github.com/Garoze/Muninn/commit/e85dbe59c8c48113f7b4ce695edb4e42737254b8))
* add CODE_OF_CONDUCT, CONTRIBUTING, and PR template ([52482a9](https://github.com/Garoze/Muninn/commit/52482a93f033f9aef3c43b9839d9001bdd5ff497))
* add design.md, cross-link from README, fix container/env var docs ([7ac6552](https://github.com/Garoze/Muninn/commit/7ac6552892b17b67f0e3e022370ada39320ccc8c))
* add documentation index and troubleshooting guide ([11b7d5b](https://github.com/Garoze/Muninn/commit/11b7d5b165185a03ab727eac15b54090526a9467))
* add ENABLED_CONFIG_SOURCES example, fix Run it section's KUBE_CONFIG_PATH/KUBECONFIG mixup ([6dc1cff](https://github.com/Garoze/Muninn/commit/6dc1cffc3585f051a5f7f090bea4692a5fd02827))
* add Jaeger trace-viewing walkthrough, rewrite Status section ([79429f2](https://github.com/Garoze/Muninn/commit/79429f296e2748404bad8eec29f0112ab1fa3778))
* add README and MIT license ([68d9b40](https://github.com/Garoze/Muninn/commit/68d9b407aacad3fd1c7cacbf5b1363f9b1294859))
* add the published chart to the README quick start ([a187a28](https://github.com/Garoze/Muninn/commit/a187a28b350a576b0eaaeb4d269f771982b0bd0e))
* add theme-adaptive logo to README ([24de047](https://github.com/Garoze/Muninn/commit/24de0470a0e7a6338d58a502bc21da13a3bf09f1))
* **adr:** remove stale ADRs, renumber and add ConfigSource/webhook records ([1482d12](https://github.com/Garoze/Muninn/commit/1482d122687bfbfc0afeaa63391777ae22f21ecd))
* align README tables with valid GFM divider syntax ([7012536](https://github.com/Garoze/Muninn/commit/7012536c429c8a34f9cde19962ff310f4cc6bda3))
* align the status, documentation and contributing sections ([4a66eca](https://github.com/Garoze/Muninn/commit/4a66eca2d4641ad48464b833fa4873457937b776))
* **app:** tighten composition-root comment for grpc.Server wiring ([50ba2ea](https://github.com/Garoze/Muninn/commit/50ba2ea52c40f472e784ebc983f36ab94e1fc857))
* bring ADR consequences in line with the code ([4439197](https://github.com/Garoze/Muninn/commit/4439197aa395045a895b1be804c4ddb5c55780a2))
* center README badges ([1b5fbcc](https://github.com/Garoze/Muninn/commit/1b5fbcc470510ac83b1859e0192fc9256c40d990))
* **config:** fix stale YAML-anchor comment in webhook deployment ([1d5bca9](https://github.com/Garoze/Muninn/commit/1d5bca9b3e1c7528d8944ae0559a8bcd3dceb429))
* correct design.md against the implementation ([fa51449](https://github.com/Garoze/Muninn/commit/fa51449132ec6c8c03666666929960ec7c39a38a))
* correct the image-loading claim and cut repetition ([c999ba5](https://github.com/Garoze/Muninn/commit/c999ba5fee43dfd02bb7e91e1b393dc34284f3ed))
* document configuration and correct the CI claim in the README ([801cb79](https://github.com/Garoze/Muninn/commit/801cb79b164055bbcfd5be3ec9f81b183d37db2b))
* document engine-agnostic image tagging, note Docker for Jaeger ([a1196f7](https://github.com/Garoze/Muninn/commit/a1196f7ce5f347b069b75702360e6eb64fb599ee))
* document in-cluster deployment decisions and Getting Started walkthrough ([e1dd9cd](https://github.com/Garoze/Muninn/commit/e1dd9cd8ad6d372e76c720a0a9da410c552cd873))
* document muninnctl usage in README ([926a865](https://github.com/Garoze/Muninn/commit/926a865abd4dedcd9a488ef953201ab746b2f1be))
* document OpenTelemetry tracing design decisions ([96dac8c](https://github.com/Garoze/Muninn/commit/96dac8c702eb639033baaf9fac6b0fc906fb2355))
* document Tenant-deletion cache semantics, envtest, and Dockerfile ([596af0c](https://github.com/Garoze/Muninn/commit/596af0c50a4f96a133c98c7099108cd74821a323))
* document the end-to-end deployment test ([b477005](https://github.com/Garoze/Muninn/commit/b477005f218d8c5ad08e00ea4bca1c1bf367821e))
* drop a broken example and trim the README further ([aeeec74](https://github.com/Garoze/Muninn/commit/aeeec74105b457fb22ac7d325cbca7157e4a0292))
* drop tenant-scoped framing from README/design/ADRs ([7391026](https://github.com/Garoze/Muninn/commit/7391026e2a0870e498af370a94a5ddb00e2dc579))
* enlarge README logo ([c0502a2](https://github.com/Garoze/Muninn/commit/c0502a2c5ad9615ba6d58b153c528dfc2e052707))
* fix a cert-manager comparison left dangling by a move ([7b0be91](https://github.com/Garoze/Muninn/commit/7b0be91d90a23d790408fecbbfd6d53941156445))
* fix stale Roadmap reference to Status section ([1690e2b](https://github.com/Garoze/Muninn/commit/1690e2b0eb8aa094080ac1de6cf54f48bb4d54d6))
* hand-curate the first release's changelog entry ([6e8779f](https://github.com/Garoze/Muninn/commit/6e8779fecde862e36611a53f951182b5cd3922e8))
* lead the run section with the command ([14c247c](https://github.com/Garoze/Muninn/commit/14c247c9f39c575cf8ef70a55abe21086f02dfff))
* move each prerequisite to the command that needs it ([1bfe55f](https://github.com/Garoze/Muninn/commit/1bfe55ff53114e5499abec592e714e89876d80f8))
* move the project layout into the contributing guide ([d947d4e](https://github.com/Garoze/Muninn/commit/d947d4e9e0978c61d0cefc346eadc4df1a524087))
* record the CSI secret delivery failure modes ([30196a4](https://github.com/Garoze/Muninn/commit/30196a449921c71efda62052213a09887eaa9d7a))
* record the CSI secret-delivery decision ([51c2bb2](https://github.com/Garoze/Muninn/commit/51c2bb2e0198bce492f30e971b6e963b1bb07266))
* reduce the README to what, why and how ([e721411](https://github.com/Garoze/Muninn/commit/e721411be85208cc7a3ee9fbf1bf99dcd1ec270d))
* reflect KeyPrefix cache-keying and drop a stale not-built claim ([05c6667](https://github.com/Garoze/Muninn/commit/05c6667062b56d9ca2f5a19cfd971678db1f49d6))
* reflect transport/grpc owning the gRPC server and Dial's optional TLS ([bd36b59](https://github.com/Garoze/Muninn/commit/bd36b59d2c19678bbb350211d0977b2dd3f0d4f3))
* reframe contributor docs as single-maintainer, fix stale references ([785141e](https://github.com/Garoze/Muninn/commit/785141e0ab7b390dc4240553a4fc679ab928a774))
* reframe no-client-library rationale as scope ([3a3b151](https://github.com/Garoze/Muninn/commit/3a3b151507f6a7e12e50f3f1b4e16ee7dfcbcf2c))
* replace ASCII diagrams with mermaid/table, drop stray divider ([c9d79ca](https://github.com/Garoze/Muninn/commit/c9d79cad28de795c4b27e6bb3582f51c5b588aee))
* replace em dashes and drop remaining history ([3a498d9](https://github.com/Garoze/Muninn/commit/3a498d9993d2dac212f0698733bb57f9c15d9f2a))
* restructure the secret-delivery section ([4ffb3f2](https://github.com/Garoze/Muninn/commit/4ffb3f2bd2b2370b8f5e96793cc0612a10a27033))
* rewrite design.md for the ConfigSource/webhook architecture ([6c518cc](https://github.com/Garoze/Muninn/commit/6c518cc3623b53ce76d711ef4a1eaa26c3ddd050))
* rewrite design.md into a full architecture document, add ADRs ([d5a7bd2](https://github.com/Garoze/Muninn/commit/d5a7bd26e972805be2dcb2c691c0f65bbf9b50d6))
* rewrite the README's framing and trim it to what a reader needs ([d672fe6](https://github.com/Garoze/Muninn/commit/d672fe6f3e5469c4c785fa44492c515cce1871a3))
* split configuration and observability reference out of the README ([e2e8034](https://github.com/Garoze/Muninn/commit/e2e8034e3325bc563e213d2a3d981d49d9552417))
* state the project's scope in the contributing section ([a993c5a](https://github.com/Garoze/Muninn/commit/a993c5ae1e967571046f87153934fa8fb6d812ce))
* **testing:** document the nightly tier ([d9b7274](https://github.com/Garoze/Muninn/commit/d9b727433ba2a21d85d0fe4da11a94bc2afb20dc))
* trim README implementation detail, link to ADRs ([c76b1ad](https://github.com/Garoze/Muninn/commit/c76b1ad4f0a949574e4684d694f94f378cc9a14b))
* update build/deploy docs and troubleshooting for ko ([d56eedc](https://github.com/Garoze/Muninn/commit/d56eedce9ff895abca9f059057d334e5b6f9de4b))
* update README for the ConfigSource/webhook architecture ([31a35c8](https://github.com/Garoze/Muninn/commit/31a35c8f7a8157971c08083c7fd35bb6b2b2125f))
* update README status for the source-filter feature ([fa5e4ef](https://github.com/Garoze/Muninn/commit/fa5e4ef9024120857d79c1f36ccbe9ea5b83b634))
* update README/CONTRIBUTING/PR template for the ConfigMap resolver ([2c70150](https://github.com/Garoze/Muninn/commit/2c7015042e0fa1b9d1002ddb02c8cb12e0c9655f))
* use declarative phrasing throughout the README ([c48fdd8](https://github.com/Garoze/Muninn/commit/c48fdd868207f71d6e1e7f38763142696eb249c6))

## [0.1.0-alpha.3](https://github.com/Garoze/Muninn/compare/v0.1.0-alpha.2...v0.1.0-alpha.3) (2026-08-15)


### Features

* **chart:** add helm-unittest, kubeconform, and ct lint to CI ([dce9ad7](https://github.com/Garoze/Muninn/commit/dce9ad70393520b9bd2a4f70d330b6a995823297))
* **chart:** add opt-in cert-manager/CSI-driver dependencies and a webhook enable toggle ([0fbca89](https://github.com/Garoze/Muninn/commit/0fbca89e2f38a9026dbc8122e9dc5cc409a92456))
* **chart:** add the Helm chart's mechanical translation and core values ([cf493a6](https://github.com/Garoze/Muninn/commit/cf493a6928d238854f4b5a7ea3940920934bbc79))
* **ci:** add a nightly workflow installing the published chart and image ([b3b80bd](https://github.com/Garoze/Muninn/commit/b3b80bd227985cfaaa4f4e8d20ccf70e3f9a7c14))
* **ci:** publish and sign the Helm chart alongside the image ([4bb9d3b](https://github.com/Garoze/Muninn/commit/4bb9d3bd3add95018c0963c93e5129b784cf3f6a))


### Bug Fixes

* **ci:** install helm explicitly before chart-testing-action ([5100e16](https://github.com/Garoze/Muninn/commit/5100e16a9966b5b84141c6b2a3b02033187a80ce))
* **ci:** register subchart repositories and fetch dependencies ([dcf6476](https://github.com/Garoze/Muninn/commit/dcf64766ec7c128a3b6c90cd0e1204139b7dce6e))
* **ci:** resync develop's release-please manifest after an official cut ([b1106e6](https://github.com/Garoze/Muninn/commit/b1106e6a2cb26e2fed90aaccb674f53dcb12b9aa))


### Documentation

* **testing:** document the nightly tier ([d9b7274](https://github.com/Garoze/Muninn/commit/d9b727433ba2a21d85d0fe4da11a94bc2afb20dc))

## [0.1.0-alpha.2](https://github.com/Garoze/Muninn/compare/v0.1.0-alpha.1...v0.1.0-alpha.2) (2026-08-15)


### Features

* **build:** add ko configuration ([46b56d9](https://github.com/Garoze/Muninn/commit/46b56d9f4b0131e579ff0ebecbd42f90085c025d))


### Bug Fixes

* **config:** point the manager/webhook Deployments at the published image ([8edb69b](https://github.com/Garoze/Muninn/commit/8edb69b3494133a1fb1eae58b3a33023408aca63))


### Documentation

* update build/deploy docs and troubleshooting for ko ([d56eedc](https://github.com/Garoze/Muninn/commit/d56eedce9ff895abca9f059057d334e5b6f9de4b))

## 0.1.0-alpha.1 (2026-08-15)

Initial release. Muninn watches labeled ConfigMaps behind a pluggable
`ConfigSource` interface, merges them into a namespace-scoped in-memory
cache, and exposes the result over a gRPC `Query`/`Resolve`/`Describe` API.

### Features

* ConfigMap aggregation via a pluggable `ConfigSource` interface, with a
  patch-based merge so one source's update never clobbers another's
* gRPC discovery API - `Query` for named keys, `Resolve` for a whole
  namespace, `Describe` for the active sources' shape
* A mutating admission webhook that delivers resolved configuration into a
  Pod as a file, kept current by a sidecar, with no client code required
* Secret references resolved via `secrets-store-csi-driver` - configuration
  carries a reference, never a value; the gRPC API stays unauthenticated by
  design and never sees secret data
* Prometheus metrics and OpenTelemetry tracing on every gRPC call and
  admission request
* `muninn` (serve/webhook/resolve) and `muninnctl` CLIs, both versioned via
  build-time `-ldflags` stamping
* Signed, published container images on GHCR (keyless cosign signing,
  verified in CI before the workflow completes)

See [`docs/design.md`](docs/design.md) and [`docs/adr/`](docs/adr/) for the
architecture and the decisions behind it.
