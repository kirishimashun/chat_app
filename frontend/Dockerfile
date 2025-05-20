# ベースイメージ
FROM node:22

# 作業ディレクトリ
WORKDIR /app

# 依存関係のインストール
COPY package*.json ./
RUN npm install

# その他全ファイルをコピー
COPY . .

# 開発用ポートを公開
EXPOSE 3000

# 開発サーバー起動
CMD ["npm", "run", "dev"]
