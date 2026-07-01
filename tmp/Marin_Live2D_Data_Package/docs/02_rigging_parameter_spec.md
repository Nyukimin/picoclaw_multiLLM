# Live2Dリギング仕様

`tables/parameter_table.csv` を正本にします。
標準的なVTube Studio連携を想定し、Live2D Cubismの一般的なParam名に寄せています。

## Deformer 推奨構成

- Root
  - Body_XYZ
    - Body_Breath
    - Arm_L / Arm_R
    - Leg_L / Leg_R
  - Head_XYZ
    - Face_Deformer
    - Eye_L / Eye_R
    - Mouth
    - Hair_Front / Hair_Side / Hair_Back
  - Physics
    - Ahoge
    - Skirt
    - Chain
    - BagCharm

## 物理設定

軽めのチビキャラなので、振れ幅は大きめ、戻りは速めが合います。
髪・チャーム・スカートの揺れは元気な印象を出すために少し弾ませます。
